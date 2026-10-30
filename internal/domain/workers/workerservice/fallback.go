package workerservice

// fallback is a service, or, where it can't start, another one.
type fallback struct {
	first, second Service
}

func (f fallback) Kind() string {
	if f.second.Running() {
		return f.second.Kind()
	}
	return f.first.Kind()
}

// Start starts the first, stopping the second if it ran before; if the
// first can't start, it starts the second, and Kind names that one.
func (f fallback) Start(spec Spec) error {
	err := f.first.Start(spec)
	if err == nil {
		f.second.Stop()
		return nil
	}
	f.first.Remove()
	if second := f.second.Start(spec); second != nil {
		return err
	}
	return nil
}

func (f fallback) Stop() error {
	f.first.Stop()
	return f.second.Stop()
}

func (f fallback) Running() bool { return f.first.Running() || f.second.Running() }

func (f fallback) Remove() error {
	f.first.Remove()
	return f.second.Remove()
}
