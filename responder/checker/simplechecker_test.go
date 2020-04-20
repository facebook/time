package checker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_SimpleCheckerListeners(t *testing.T) {
	randomNumber := int64(100500)

	checker := SimpleChecker{realListeners: randomNumber}
	checker.IncListeners()
	assert.Equal(t, checker.realListeners, randomNumber+1)

	checker.DecListeners()
	assert.Equal(t, checker.realListeners, randomNumber)
}

func Test_SimpleCheckerWorkers(t *testing.T) {
	randomNumber := int64(100500)

	checker := SimpleChecker{realWorkers: randomNumber}
	checker.IncWorkers()
	assert.Equal(t, checker.realWorkers, randomNumber+1)

	checker.DecWorkers()
	assert.Equal(t, checker.realWorkers, randomNumber)
}

func Test_SimpleCheckListeners(t *testing.T) {
	checker := SimpleChecker{ExpectedListeners: 1}
	checker.IncListeners()

	assert.Nil(t, checker.checkListeners())
}

func Test_CheckListenersFail(t *testing.T) {
	checker := SimpleChecker{ExpectedListeners: 1}
	checker.IncListeners()
	checker.DecListeners()

	assert.Equal(t, checker.checkListeners(), errSimpleCheckerWrongAmountListeners)
}

func Test_SimpleCheckerCheckWorkers(t *testing.T) {
	checker := SimpleChecker{ExpectedWorkers: 1}
	checker.IncWorkers()

	assert.Nil(t, checker.checkWorkers())
}

func Test_SimpleCheckerCheckWorkersFail(t *testing.T) {
	checker := SimpleChecker{ExpectedWorkers: 1}
	checker.IncWorkers()
	checker.DecWorkers()

	assert.Equal(t, checker.checkWorkers(), errSimpleCheckerWrongAmountWorkers)
}
