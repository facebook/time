/*
Copyright (c) Facebook, Inc. and its affiliates.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package daemon

// Warm-up tolerance factor for the GradualWindow feature (T274511655).
//
// The published window is W = mean(m) + k*stddev(m). In steady state k is the constant 4. Right after a
// restart the ring is nearly empty, so stddev is estimated from very few samples and is unreliable
// (biased low, 0 at n=1), and a naive window could be too narrow exactly when we are least sure.
// GradualWindow replaces the constant 4 with a sample-count factor k(n): wide at small n, converging to
// exactly 4 at the full ring.
//
// Two parameters drive k(n):
//   - z_p (coverageZP) is how wide: the sigma count, fixed at 4 (the steady-state coverage clients rely on).
//   - gamma (warmupConfidence) is how conservative we are estimating that width from few samples. Higher
//     gamma widens warm-up only; it vanishes at the full ring, where k is anchored to 4 for any gamma.
//
// k(n) is the one-sided normal tolerance factor, the noncentral-t quantile
//
//	k(n) = Q_noncentral_t(gamma; df = n-1, noncentrality = sqrt(n)*z_p) / sqrt(n)
//
// finite for every n >= 2 (n=1 is skipped: no spread from a single point). It depends only on n, so the
// table k[2..RingSize] is precomputed once in EvalAndValidate and the hot path is a slice lookup.
//
// That quantile has no closed form, and no single numerical method is accurate over our whole n range, so
// exactFactor uses two that agree where they overlap: the AS 243 noncentral-t series (Lenth 1989, Applied
// Statistics), accurate at small n but underflowing its exp(-delta^2/2) term at the full-ring noncentrality
// (delta=40); and a numerical integral over the sample-stddev (chi) distribution, stable there but weaker
// at the smallest n. Both are cross-checked in tests against published tolerance tables and each other.

import "math"

// The operating point: the two parameters that define the window. Both hardcoded, not configurable.
const (
	// coverageZP is z_p, the steady multiplier k converges to (the 4 in MathDefaultW, where the 4-sigma SLA
	// is explained). The warm-up tolerance table is anchored to it.
	coverageZP = 4.0

	// warmupConfidence is gamma: how conservative the warm-up factor k(n) is while the ring has few
	// samples. Higher = wider early window; anchored away at the full ring (k(RingSize) == coverageZP for
	// any value). Hardcoded like coverageZP; 0.9999 is deliberately conservative and stays finite below 1.
	warmupConfidence = 0.9999
)

// Numerical-method constants for computing k(n) (see exactFactor); tuning only, not the operating point.
const (
	// ncTSwitchDelta is the crossover, in units of noncentrality delta=sqrt(n)*z_p, between exactFactor's
	// two routines: below it the AS 243 series is accurate; at/above it that series underflows its
	// exp(-delta^2/2) term (delta=40 at the n=100 anchor), so exactFactor switches to the chi integral.
	ncTSwitchDelta = 36.0

	// chiIntegralPanels is the Simpson's-rule resolution (even panel count) of exactFactor's chi integral
	// (its large-noncentrality routine): higher is more accurate but slower. 4000 over-resolves the smooth
	// integrand; computed once at config parse.
	chiIntegralPanels = 4000

	// Each solver below (continued fraction, series, bisection) loops until its convergence tolerance is
	// met, bounded by a max iteration count as a safety cap. These name those per-routine caps and tolerances.
	betacfMaxIter       = 500   // betacf continued-fraction iterations
	betacfTol           = 1e-15 // betacf convergence
	nctCDFMaxIter       = 1000  // nctCDF (AS 243) series terms
	nctCDFTol           = 1e-14 // nctCDF convergence
	nctPPFMaxIter       = 200   // nctPPF bisection iterations
	nctPPFTol           = 1e-10 // nctPPF bisection convergence (relative)
	factorBisectMaxIter = 100   // exactFactorIntegral k-bisection iterations
	factorBisectTol     = 1e-12 // exactFactorIntegral k-bisection convergence (relative)
)

// lgamma returns log|Gamma(x)| (the sign is always + for the positive arguments used here).
func lgamma(x float64) float64 {
	v, _ := math.Lgamma(x)
	return v
}

// normCDF is the standard-normal CDF Phi(z).
func normCDF(z float64) float64 {
	return 0.5 * math.Erfc(-z/math.Sqrt2)
}

// betacf is the continued fraction used by regIncBeta (Numerical Recipes "betacf", Lentz's method).
func betacf(a, b, x float64) float64 {
	const tiny = 1e-300
	qab := a + b
	qap := a + 1.0
	qam := a - 1.0
	c := 1.0
	d := 1.0 - qab*x/qap
	if math.Abs(d) < tiny {
		d = tiny
	}
	d = 1.0 / d
	h := d
	for m := 1; m < betacfMaxIter; m++ {
		mf := float64(m)
		m2 := 2.0 * mf
		aa := mf * (b - mf) * x / ((qam + m2) * (a + m2))
		d = 1.0 + aa*d
		if math.Abs(d) < tiny {
			d = tiny
		}
		c = 1.0 + aa/c
		if math.Abs(c) < tiny {
			c = tiny
		}
		d = 1.0 / d
		h *= d * c
		aa = -(a + mf) * (qab + mf) * x / ((a + m2) * (qap + m2))
		d = 1.0 + aa*d
		if math.Abs(d) < tiny {
			d = tiny
		}
		c = 1.0 + aa/c
		if math.Abs(c) < tiny {
			c = tiny
		}
		d = 1.0 / d
		de := d * c
		h *= de
		if math.Abs(de-1.0) < betacfTol {
			break
		}
	}
	return h
}

// regIncBeta is the regularized incomplete beta function I_x(a,b). Used by the AS243 noncentral-t CDF.
func regIncBeta(a, b, x float64) float64 {
	if x <= 0 {
		return 0.0
	}
	if x >= 1 {
		return 1.0
	}
	lbeta := lgamma(a+b) - lgamma(a) - lgamma(b)
	front := math.Exp(lbeta + a*math.Log(x) + b*math.Log(1.0-x))
	if x < (a+1.0)/(a+b+2.0) {
		return front * betacf(a, b, x) / a
	}
	return 1.0 - front*betacf(b, a, 1.0-x)/b
}

// nctCDF is the noncentral-t CDF P(T' <= t) for df=nu, noncentrality=delta (Lenth 1989, AS 243).
// Accurate for small nu; its leading exp(-delta^2/2) term underflows once delta >~ ncTSwitchDelta,
// which is exactly why exactFactor switches to the integral above that threshold.
func nctCDF(t, nu, delta float64) float64 {
	if t < 0 {
		return 1.0 - nctCDF(-t, nu, -delta)
	}
	x := t * t / (t*t + nu)
	if x <= 0 {
		return normCDF(-delta)
	}
	en := 1.0
	p := 0.5 * math.Exp(-0.5*delta*delta)
	q := math.Sqrt(2.0/math.Pi) * p * delta
	s := 0.5 - p
	a := 0.5
	b := 0.5 * nu
	rxb := math.Pow(1.0-x, b)
	lbeta := lgamma(a) + lgamma(b) - lgamma(a+b)
	xodd := regIncBeta(a, b, x)
	godd := 2.0 * rxb * math.Exp(a*math.Log(x)-lbeta)
	xeven := 1.0 - rxb
	geven := b * x * rxb
	tnc := p*xodd + q*xeven
	for range nctCDFMaxIter {
		a += 1.0
		xodd -= godd
		xeven -= geven
		godd *= x * (a + b - 1.0) / a
		geven *= x * (a + b - 0.5) / (a + 0.5)
		p *= delta * delta / (2.0 * en)
		q *= delta * delta / (2.0*en + 1.0)
		s -= p
		en += 1.0
		tnc += p*xodd + q*xeven
		if 2.0*s*(xodd-godd) < nctCDFTol {
			break
		}
	}
	return tnc + normCDF(-delta)
}

// nctPPF is the gamma-quantile of the noncentral-t distribution (root-find on nctCDF). Used for the
// delta < ncTSwitchDelta branch of exactFactor.
func nctPPF(gamma, nu, delta float64) float64 {
	// Phase 1: grow hi until it brackets the quantile (CDF(hi) >= gamma); give up (Inf) if it never does.
	lo, hi := -10.0, 100.0
	for nctCDF(hi, nu, delta) < gamma {
		hi *= 2.0
		if hi > 1e12 {
			return math.Inf(1)
		}
	}
	// Phase 2: bisection on the monotone CDF.
	for range nctPPFMaxIter {
		mid := 0.5 * (lo + hi)
		if nctCDF(mid, nu, delta) < gamma {
			lo = mid
		} else {
			hi = mid
		}
		if hi-lo < nctPPFTol*math.Max(1.0, hi) {
			break
		}
	}
	return 0.5 * (lo + hi)
}

// chiLogPDF is the log density of the chi distribution with nu degrees of freedom.
func chiLogPDF(w, nu float64) float64 {
	return (nu-1.0)*math.Log(w) - 0.5*w*w - ((nu/2.0-1.0)*math.Log(2.0) + lgamma(nu/2.0))
}

// exactFactorIntegral computes the same tolerance factor as the noncentral-t quantile, but by integrating
// over the chi distribution of the sample stddev. For a candidate k the integral gives the achieved
// confidence, then it bisects on k to hit gamma:
//
//	gamma = E_W[ Phi( sqrt(n)*(k*W/sqrt(nu) - zp) ) ],  W ~ chi_nu.
//
// It has no exp(-delta^2/2) term, so it is stable where AS 243 underflows (large delta, e.g. the
// RingSize=100 anchor at delta=40).
func exactFactorIntegral(n, zp, gamma float64) float64 {
	nu := n - 1.0
	sn := math.Sqrt(n)
	isnu := 1.0 / math.Sqrt(nu)

	lo := 0.0
	hi := math.Sqrt(nu) + 16.0 // chi_nu concentrates near sqrt(nu); +16 is many sd of headroom
	panels := chiIntegralPanels
	h := (hi - lo) / float64(panels)
	nodes := make([]float64, panels+1)
	weights := make([]float64, panels+1)
	for i := range nodes {
		w := lo + float64(i)*h
		var sc float64
		switch {
		case i == 0 || i == panels:
			sc = 1.0
		case i%2 == 1:
			sc = 4.0
		default:
			sc = 2.0
		}
		var pdf float64
		if w > 0.0 {
			pdf = math.Exp(chiLogPDF(w, nu))
		}
		nodes[i] = w
		weights[i] = sc * (h / 3.0) * pdf
	}

	g := func(k float64) float64 {
		s := 0.0
		for i := range nodes {
			s += weights[i] * normCDF(sn*(k*nodes[i]*isnu-zp))
		}
		return s
	}

	klo, khi := 0.0, 4.0*(zp+1.0)
	for g(khi) < gamma {
		khi *= 2.0
		if khi > 1e6 {
			return math.Inf(1)
		}
	}
	for range factorBisectMaxIter {
		mid := 0.5 * (klo + khi)
		if g(mid) < gamma {
			klo = mid
		} else {
			khi = mid
		}
		if khi-klo < factorBisectTol*math.Max(1.0, khi) {
			break
		}
	}
	return 0.5 * (klo + khi)
}

// exactFactor numerically computes the one-sided normal tolerance factor t'_{n-1, sqrt(n)*zp}(gamma)/sqrt(n)
// for n >= 2 (NaN for n < 2, the genuine boundary). It is hybrid for numerical stability across the whole
// domain: the AS 243 noncentral-t quantile for delta < ncTSwitchDelta (accurate at tiny nu), and the
// chi-density integral for delta >= ncTSwitchDelta (where AS 243 underflows). The two methods agree to
// ~1e-3 or better in their overlap (see TestExactFactorMethodsAgree).
func exactFactor(n, zp, gamma float64) float64 {
	if n < 2 {
		return math.NaN()
	}
	delta := math.Sqrt(n) * zp
	if delta < ncTSwitchDelta {
		return nctPPF(gamma, n-1.0, delta) / math.Sqrt(n)
	}
	return exactFactorIntegral(n, zp, gamma)
}

// precomputeGradualWindowFactors builds the data-independent warm-up factor table k[2..ringSize], used
// as W = mean(m,n) + k(n)*stddev(m,n) during warm-up. k(n) is the one-sided noncentral-t tolerance
// factor (z_p = coverageZP) scaled so k(ringSize) is exactly coverageZP (4.0), so the full-ring window
// is the exact same number as the legacy "mean(m,N) + 4*stddev(m,N)". Indices 0 and 1 are left zero
// (n=1 is skipped: its stddev is 0 so there is no margin to scale). Built once at config-parse; the hot
// path only does a slice lookup.
func precomputeGradualWindowFactors(ringSize int, gamma float64) []float64 {
	out := make([]float64, ringSize+1)
	if ringSize < 2 {
		return out
	}
	fN := exactFactor(float64(ringSize), coverageZP, gamma)
	for n := 2; n <= ringSize; n++ {
		if n == ringSize {
			// Anchor: pin to exactly coverageZP so the full-ring window is the exact same number as
			// today, independent of any floating-point error in fN.
			out[n] = coverageZP
			continue
		}
		out[n] = coverageZP * exactFactor(float64(n), coverageZP, gamma) / fN
	}
	return out
}
