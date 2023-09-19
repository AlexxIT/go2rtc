package streams

func (s *Stream) Publish(url string) error {
	cons, run, err := GetConsumer(url)
	if err != nil {
		return err
	}

	if err = s.AddConsumer(cons); err != nil {
		return err
	}

	go func() {
		run()
		s.RemoveConsumer(cons)
	}()

	return nil
}
