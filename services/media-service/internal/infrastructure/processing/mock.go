package processing

import (
	"context"

	"github.com/qsyy0921/IM/services/media-service/internal/types"
)

type MockScanner struct{}

func NewMockScanner() MockScanner {
	return MockScanner{}
}

func (MockScanner) Scan(context.Context, types.MediaAsset) error {
	return nil
}

type MockThumbnailer struct{}

func NewMockThumbnailer() MockThumbnailer {
	return MockThumbnailer{}
}

func (MockThumbnailer) GenerateThumbnail(context.Context, types.MediaAsset) error {
	return nil
}

type MockTranscoder struct{}

func NewMockTranscoder() MockTranscoder {
	return MockTranscoder{}
}

func (MockTranscoder) Transcode(context.Context, types.MediaAsset) error {
	return nil
}
