package logs

import (
	"context"
	"fmt"
	"sync"
	"text/tabwriter"
)

type key int

const scoreboardKey key = 0

// Scoreboard holds retry statistics
type Scoreboard struct {
	mu     sync.Mutex
	Counts []int // index 0 = success on first try, 1 = after 1 retry, ..., last = failed
	log    Logger
}

// New creates a new scoreboard
// maxRetryCount = how many retries you allow before giving up
func NewSB(maxRetryCount int, log Logger) *Scoreboard {
	return &Scoreboard{
		Counts: make([]int, maxRetryCount+2), // +2: one for success-on-first, one for final failure
		log:    log,
	}
}

// Increment records a result after `retryCount` attempts
// retryCount == 0 → succeeded on first try
// retryCount == max → final failure
func (s *Scoreboard) IncrementSB(retryCount int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if retryCount < 0 {
		retryCount = 0
	}
	if retryCount >= len(s.Counts)-1 {
		retryCount = len(s.Counts) - 1 // final failure bucket
	}
	s.Counts[retryCount]++
}

// Print the beautiful table at the end
func (s *Scoreboard) PrintSB() {
	s.mu.Lock()
	defer s.mu.Unlock()

	total := 0
	for _, c := range s.Counts {
		total += c
	}
	if total == 0 {
		return
	}

	s.log.Println("\n\nSubmission Results")
	w := tabwriter.NewWriter(s.log.Writer(), 0, 0, 2, ' ', 0)

	for i, count := range s.Counts {
		if i == 0 {
			fmt.Fprintf(w, "Success (no retry)\t%d\n", count)
		} else if i == 1 {
			fmt.Fprintf(w, "Success after %d retry\t%d\n", i, count)
		} else if i < len(s.Counts)-1 {
			fmt.Fprintf(w, "Success after %d retries\t%d\n", i, count)
		} else {
			fmt.Fprintf(w, "Failed (all retries exhausted)\t%d\n", count)
		}
	}
	fmt.Fprintf(w, "TOTAL\t%d\n", total)
	w.Flush()
}

// Context helpers — so you don't have to pass scoreboard around

func NewSBContext(parent context.Context, sb *Scoreboard) context.Context {
	return context.WithValue(parent, scoreboardKey, sb)
}

func FromSBContext(ctx context.Context) (*Scoreboard, error) {
	if sb, ok := ctx.Value(scoreboardKey).(*Scoreboard); ok {
		return sb, nil
	}
	return nil, fmt.Errorf("Scoreboard is not of type Scoreboard")
}
