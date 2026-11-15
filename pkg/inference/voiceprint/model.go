package voiceprint

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ygelfand/libcountertop/pkg/network/releases"
)

const (
	modelFolder = "voiceprint/b1-1"
	modelName   = "voiceprint-b1.tflite"
	modelSum    = "d4e4f54f0b82d3e4ab037a116e0e7dc82b1ffbd4ec658947cd5dd4b8adcd174d"
	modelSize   = 9316704
)

func Fetch(ctx context.Context, dir string) ([]byte, error) {
	dest := filepath.Join(dir, modelName)
	if info, err := os.Stat(dest); err != nil || info.Size() != modelSize {
		url, err := releases.File(modelFolder, modelName)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("voiceprint: %w", err)
		}
		if err := releases.Download(ctx, url, modelSum, dest, modelSize); err != nil {
			return nil, fmt.Errorf("voiceprint: %w", err)
		}
	}
	return os.ReadFile(dest)
}
