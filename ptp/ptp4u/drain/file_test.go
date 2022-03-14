package drain

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheck(t *testing.T) {
	file, err := ioutil.TempFile("", "")
	require.NoError(t, err)

	check := &FileDrain{FileName: file.Name()}
	require.True(t, check.Check())

	os.Remove(file.Name())
	require.False(t, check.Check())
}
