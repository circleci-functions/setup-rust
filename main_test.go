package main

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestSplitList(t *testing.T) {
	assert.DeepEqual(t, splitList(" rustfmt, clippy ,,"), []string{"rustfmt", "clippy"})
	assert.Assert(t, splitList("") == nil)
}
