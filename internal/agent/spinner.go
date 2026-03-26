package agent

import (
	"fmt"
	"io"
	"sync"
	"time"
)

var brailleFrames = []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}

// Spinner shows an animated indicator during LLM calls.
type Spinner struct {
	mu       sync.Mutex
	out      io.Writer
	stop     chan struct{}
	stopped  chan struct{}
	running  bool
	provider string
	model    string
}

// NewSpinner creates a spinner that writes to out.
func NewSpinner(out io.Writer) *Spinner {
	return &Spinner{out: out}
}

// Start begins the spinner animation showing the provider and model.
func (s *Spinner) Start(provider, model string) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.provider = provider
	s.model = model
	s.stop = make(chan struct{})
	s.stopped = make(chan struct{})
	s.running = true
	s.mu.Unlock()

	go s.run()
}

// Stop halts the spinner and clears the line.
func (s *Spinner) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.stop)
	s.mu.Unlock()
	<-s.stopped
	// Clear spinner line
	fmt.Fprintf(s.out, "\r\033[K")
}

func (s *Spinner) run() {
	defer close(s.stopped)
	start := time.Now()
	frame := 0
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			elapsed := time.Since(start).Truncate(time.Second)
			label := s.provider
			if s.model != "" && s.model != "auto" {
				label = fmt.Sprintf("%s [%s]", s.provider, s.model)
			}
			if label == "" {
				label = "thinking"
			}
			fmt.Fprintf(s.out, "\r\033[36m%c\033[0m %s \033[2m(%s)\033[0m\033[K",
				brailleFrames[frame%len(brailleFrames)], label, elapsed)
			frame++
		}
	}
}
