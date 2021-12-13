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

package checker

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSimpleCheckerListeners(t *testing.T) {
	randomNumber := int64(100500)

	checker := SimpleChecker{realListeners: randomNumber}
	checker.IncListeners()
	require.Equal(t, checker.realListeners, randomNumber+1)

	checker.DecListeners()
	require.Equal(t, checker.realListeners, randomNumber)
}

func TestSimpleCheckerWorkers(t *testing.T) {
	randomNumber := int64(100500)

	checker := SimpleChecker{realWorkers: randomNumber}
	checker.IncWorkers()
	require.Equal(t, checker.realWorkers, randomNumber+1)

	checker.DecWorkers()
	require.Equal(t, checker.realWorkers, randomNumber)
}

func TestSimpleCheckListeners(t *testing.T) {
	checker := SimpleChecker{ExpectedListeners: 1}
	checker.IncListeners()

	require.Nil(t, checker.checkListeners())
}

func TestCheckListenersFail(t *testing.T) {
	checker := SimpleChecker{ExpectedListeners: 1}
	checker.IncListeners()
	checker.DecListeners()

	require.Equal(t, checker.checkListeners(), errSimpleCheckerWrongAmountListeners)
}

func TestSimpleCheckerCheckWorkers(t *testing.T) {
	checker := SimpleChecker{ExpectedWorkers: 1}
	checker.IncWorkers()

	require.Nil(t, checker.checkWorkers())
}

func TestSimpleCheckerCheckWorkersFail(t *testing.T) {
	checker := SimpleChecker{ExpectedWorkers: 1}
	checker.IncWorkers()
	checker.DecWorkers()

	require.Equal(t, checker.checkWorkers(), errSimpleCheckerWrongAmountWorkers)
}
