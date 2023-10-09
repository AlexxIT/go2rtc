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

func Publish(stream *Stream, destination any) {
	switch v := destination.(type) {
	case string:
		if err := stream.Publish(v); err != nil {
			log.Error().Err(err).Caller().Send()
		}
	case []any:
		for _, v := range v {
			Publish(stream, v)
		}
	}
}
