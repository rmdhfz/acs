package simcpe

import (
	"fmt"
	"sync/atomic"
	"time"
)

var idCounter int64

// nextID membuat SOAP Header ID unik untuk envelope keluar.
func nextID() string {
	return fmt.Sprintf("simcpe-%d-%d", time.Now().UnixNano(), atomic.AddInt64(&idCounter, 1))
}
