package types

import (
	"github.com/justinbarrick/farm/pkg/cache/file"
	"github.com/justinbarrick/farm/pkg/cache/s3"
	"github.com/justinbarrick/farm/pkg/job"
)

type Config struct {
	Jobs   []*job.Job   `hcl:"job,block"`
	Cache  *CacheConfig `hcl:"cache,block"`
	Engine *string      `hcl:"engine"`
	Workspace *string   `hcl:"workspace"`
}

type CacheConfig struct {
	S3   *s3cache.S3Cache     `hcl:"s3,block"`
	File *filecache.FileCache `hcl:"file,block"`
}
