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
