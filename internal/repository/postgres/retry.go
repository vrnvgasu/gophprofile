package postgres

import "github.com/vrnvgasu/gophprofile/pkg/retry"

// withRetry повторяет операцию при временных ошибках соединения с базой.
func withRetry(fn func() error, classifier *postgresErrorClassifier) error {
	return retry.Retry(func() error {
		err := fn()
		if err == nil {
			return nil
		}

		if classifier != nil && classifier.classify(err) == nonRetriable {
			return err
		}

		return retry.NewRetryableError(err)
	})
}
