// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package flow

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"

	"github.com/craftslab/lintflow/lint"
	"github.com/craftslab/lintflow/proto"
	"github.com/craftslab/lintflow/review"
	"github.com/craftslab/lintflow/runtime"
)

type Flow interface {
	Run(string) ([]proto.Format, error)
}

type Config struct {
	Lint   lint.Lint
	Review review.Review
}

type flow struct {
	cfg *Config
}

func New(_ context.Context, cfg *Config) Flow {
	return &flow{
		cfg: cfg,
	}
}

func DefaultConfig() *Config {
	return &Config{}
}

func (f *flow) Run(commit string) ([]proto.Format, error) {
	var err error
	var ret []proto.Format

	buf, err := runtime.Run(f.routine, []interface{}{commit})
	if err != nil {
		return nil, errors.Wrap(err, "failed to run")
	}

	err = nil

	for _, val := range buf {
		if val == nil {
			err = errors.New("invalid data")
			break
		}
		if len(val.([]proto.Format)) != 0 {
			ret = append(ret, val.([]proto.Format)...)
		}
	}

	return ret, err
}

func (f *flow) routine(data interface{}) interface{} {
	d, _ := os.Getwd()
	t := time.Now()
	root := filepath.Join(d, "gerrit-"+t.Format("2006-01-02"))

	commit := data.(string)

	dir, repo, files, err := f.cfg.Review.Fetch(root, commit)
	defer func() { _ = f.cfg.Review.Clean(root) }()
	if err != nil {
		log.Println(err)
		return nil
	}

	buf, err := f.cfg.Lint.Run(dir, repo, files)
	if err != nil {
		log.Println(err)
		return nil
	}

	if buf == nil {
		return []proto.Format{}
	}

	if err := f.cfg.Review.Vote(commit, buf); err != nil {
		log.Println(err)
		return nil
	}

	return buf
}
