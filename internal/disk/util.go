package disk

import (
	"time"
	"unsafe"
)

func unsafePointer[T any](p *T) unsafe.Pointer { return unsafe.Pointer(p) }

func sleep100ms() { time.Sleep(100 * time.Millisecond) }
