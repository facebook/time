package checker

import (
	"errors"
	"sync/atomic"

	log "github.com/sirupsen/logrus"
)

var (
	errSimpleCheckerWrongAmountListeners = errors.New("wrong amount of listeners is up")
	errSimpleCheckerWrongAmountWorkers   = errors.New("wrong amount of workers is up")
)

// SimpleChecker is a datasruct containing monitoring info for internal healthchecks
type SimpleChecker struct {
	// ExpectedListeners is number of listeners we expect to run
	ExpectedListeners int64
	realListeners     int64

	// ExpectedWorkers is number of workers we expect to run
	ExpectedWorkers int64
	realWorkers     int64
}

// IncListeners thread-safely increases number of workers to monitor
func (s *SimpleChecker) IncListeners() {
	atomic.AddInt64(&s.realListeners, 1)
}

// DecListeners thread-safely increases number of workers to monitor
func (s *SimpleChecker) DecListeners() {
	atomic.AddInt64(&s.realListeners, -1)
}

// IncWorkers thread-safely increases number of workers to monitor
func (s *SimpleChecker) IncWorkers() {
	atomic.AddInt64(&s.realWorkers, 1)
}

// DecWorkers thread-safely increases number of workers to monitor
func (s *SimpleChecker) DecWorkers() {
	atomic.AddInt64(&s.realWorkers, -1)
}

// Check is a method which performs basic validations that responder is alive
func (s *SimpleChecker) Check() error {
	var err error
	err = s.checkListeners()
	if err != nil {
		return err
	}

	err = s.checkWorkers()
	if err != nil {
		return err
	}

	return nil
}

// CheckListeners if all ExpectedListeners are alive
func (s *SimpleChecker) checkListeners() error {
	log.Debug("[Checker] checking listeners")
	if s.ExpectedListeners != s.realListeners {
		return errSimpleCheckerWrongAmountListeners
	}
	return nil
}

// CheckWorkers if all ExpectedListeners are alive
func (s *SimpleChecker) checkWorkers() error {
	log.Debug("[Checker] checking workers")
	if s.ExpectedWorkers != s.realWorkers {
		return errSimpleCheckerWrongAmountWorkers
	}
	return nil
}
