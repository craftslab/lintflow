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

package lint

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/grpc"

	"github.com/craftslab/lintflow/config"
	"github.com/craftslab/lintflow/proto"
)

type Lint interface {
	Run(root string, files []string) ([]proto.Format, error)
}

type Config struct {
	Lints []config.Lint
}

type lint struct {
	cfg *Config
}

func New(cfg *Config) Lint {
	return &lint{
		cfg: cfg,
	}
}

func DefaultConfig() *Config {
	return &Config{}
}

func (l *lint) Run(root string, files []string) ([]proto.Format, error) {
	type result struct {
		data []proto.Format
		err  error
	}

	ch := make(chan result, len(l.cfg.Lints))

	for _, val := range l.cfg.Lints {
		go func(root string, files []string, val config.Lint) {
			f := l.filter(val.Filter, files)
			if len(f) != 0 {
				m, e := l.marshal(root, f)
				if e != nil {
					ch <- result{nil, errors.Wrap(e, "failed to marshal")}
				}
				r, e := l.routine(val.Host, val.Port, m)
				if e != nil {
					ch <- result{nil, errors.Wrap(e, "failed to routine")}
				}
				ch <- result{r, nil}
			} else {
				ch <- result{[]proto.Format{}, nil}
			}
		}(root, files, val)
	}

	var ret []proto.Format

	for range l.cfg.Lints {
		r := <-ch
		if r.err != nil {
			return nil, r.err
		}
		if len(r.data) != 0 {
			ret = append(ret, r.data...)
		}
	}

	return ret, nil
}

func (l *lint) filter(f config.Filter, data []string) []string {
	helper := func(data string) bool {
		match := false
		for _, val := range f.Include.Extension {
			if val == filepath.Ext(strings.TrimSuffix(data, proto.Base64Content)) {
				match = true
				break
			}
		}
		return match
	}

	var buf []string

	for _, val := range data {
		if helper(val) {
			buf = append(buf, val)
		}
	}

	return buf
}

func (l *lint) marshal(root string, data []string) ([]byte, error) {
	helper := func(name string) (string, error) {
		fi, err := os.Open(name)
		if err != nil {
			return "", errors.Wrap(err, "failed to open")
		}
		defer func() { _ = fi.Close() }()
		buf, err := ioutil.ReadAll(fi)
		if err != nil {
			return "", errors.Wrap(err, "failed to readall")
		}
		return string(buf), nil
	}

	var err error
	buf := map[string]string{}

	for _, val := range data {
		if val == "" {
			err = errors.New("invalid data")
			break
		}
		buf[val], err = helper(filepath.Join(root, val))
		if err != nil {
			break
		}
	}

	if len(buf) == 0 {
		return nil, errors.New("invalid data")
	}

	if err != nil {
		return nil, errors.Wrap(err, "failed to read")
	}

	ret, err := json.Marshal(buf)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal")
	}

	return ret, nil
}

func (l *lint) routine(host string, port int, data []byte) ([]proto.Format, error) {
	helper := func(data string) ([]proto.Format, error) {
		var buf map[string][]proto.Format
		if err := json.Unmarshal([]byte(data), &buf); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal")
		}
		var ret []proto.Format
		for _, val := range buf {
			ret = append(ret, val...)
		}
		return ret, nil
	}

	conn, err := grpc.Dial(host+":"+strconv.Itoa(port), grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, errors.Wrap(err, "failed to dial")
	}
	defer func() { _ = conn.Close() }()

	client := NewLintProtoClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	reply, err := client.SendLint(ctx, &LintRequest{Message: string(data)})
	if err != nil {
		return nil, errors.Wrap(err, "failed to send")
	}

	buf, err := helper(reply.GetMessage())
	if err != nil {
		return nil, errors.Wrap(err, "failed to get")
	}

	return buf, nil
}
