package cache

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCacheOptions(t *testing.T) {
	o := newOptions()
	assert.Equal(t, defaultExpiry, o.Expiry)
	assert.Equal(t, defaultNotFoundExpiry, o.NotFoundExpiry)
	assert.Nil(t, o.Invalidations)

	queue := NewInvalidationQueue()
	o = newOptions(WithInvalidationQueue(queue))
	assert.Same(t, queue, o.Invalidations)
	assert.Equal(t, defaultExpiry, o.Expiry)
}
