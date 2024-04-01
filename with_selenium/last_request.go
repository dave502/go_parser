package with_selenium

import (
	"sync"
	"time"
)

// struct for tracking request frequency
// not implemented !
type LastRequest struct {
	time time.Time
	sync.Mutex
}

func (r *LastRequest) setNowTime() {
	r.Lock()
	defer r.Unlock()
	r.time = time.Now()
}

func (r *LastRequest) getTime() time.Time {
	r.Lock()
	defer r.Unlock()
	return r.time
}

func (r *LastRequest) secondsFromLastTime() float64 {
	r.Lock()
	defer r.Unlock()
	return time.Since(r.time).Seconds()
}
