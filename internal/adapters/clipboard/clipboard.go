package clipboard

import (
	"fmt"

	"github.com/atotto/clipboard"
)

func Copy(value string) error {
	if value == "" {
		return fmt.Errorf("nothing to copy")
	}
	if err := clipboard.WriteAll(value); err != nil {
		return fmt.Errorf("clipboard write failed: %w", err)
	}
	return nil
}
