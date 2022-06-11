package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfig(t *testing.T) {
	config := DefaultConfig()
	assert.Equal(t, config.HomeDir, "/root/.oppyChain/config")
}
