package stalker

type delayTracker struct {
	size int
	pos  int
	mean int
	nums []int

	min int
	max int
}

func newDelayTracker(size int, min, max int) *delayTracker {
	return &delayTracker{
		size: size,
		min:  min,
		max:  max,
	}
}

func (t *delayTracker) Add(d int) {
	if d <= t.min {
		d = t.min
	} else if d >= t.max {
		d = t.max
	}
	if len(t.nums) < t.size {
		// we are not yet full, so we can just append and calculate a new mean
		// and set new positions
		t.mean = (t.mean*int(len(t.nums)) + d) / (len(t.nums) + 1)
		t.nums = append(t.nums, d)
		t.pos = len(t.nums)
	} else {
		// ok, we are full on the ringbuffer, so cycle back to the beginning
		t.pos++
		if t.pos >= t.size {
			t.pos = 0
		}
		// remove the oldest element and replace it with the new one
		old := t.nums[t.pos]
		t.nums[t.pos] = d
		newMean := (t.mean*int(t.size) - old + d) / int(t.size)
		t.mean = newMean
	}
}

func (t *delayTracker) Mean() int {
	return t.mean
}
