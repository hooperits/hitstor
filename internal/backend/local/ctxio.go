package local

import (
	"context"
	"io"
)

func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader) error {
	if ctx == nil {
		_, err := io.Copy(dst, src)
		return err
	}
	buf := make([]byte, 32*1024)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			if _, err := dst.Write(buf[:n]); err != nil {
				return err
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}
