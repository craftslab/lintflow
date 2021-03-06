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

package review

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/craftslab/lintflow/config"
	"github.com/craftslab/lintflow/proto"
)

type gerrit struct {
	r config.Review
}

func (g *gerrit) Clean(name string) error {
	if err := os.RemoveAll(name); err != nil {
		return errors.Wrap(err, "failed to clean")
	}

	return nil
}

func (g *gerrit) Fetch(commit string) (rname string, flist []string, emsg error) {
	helper := func(dir, file, data string) error {
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			return errors.Wrap(err, "failed to mkdir")
		}
		f, err := os.Create(filepath.Join(dir, file))
		if err != nil {
			return errors.Wrap(err, "failed to create")
		}
		defer func() {
			_ = f.Close()
		}()
		b := bufio.NewWriter(f)
		if _, err := b.WriteString(data); err != nil {
			return errors.Wrap(err, "failed to write")
		}
		defer func() {
			_ = b.Flush()
		}()
		return nil
	}

	// Set root
	d, err := os.Getwd()
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to getwd")
	}

	t := time.Now()
	root := filepath.Join(d, "gerrit-"+t.Format("2006-01-02"))

	// Query commit
	r, err := g.get(g.urlQuery("commit:"+commit, []string{"CURRENT_FILES", "CURRENT_REVISION"}, 0))
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to query")
	}

	ret, err := g.unmarshalList(r)
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to unmarshal")
	}

	revisions := ret["revisions"].(map[string]interface{})
	current := revisions[ret["current_revision"].(string)].(map[string]interface{})

	changeNum := int(ret["_number"].(float64))
	revisionNum := int(current["_number"].(float64))

	path := filepath.Join(root, strconv.Itoa(changeNum), ret["current_revision"].(string))

	// Get patch
	buf, err := g.get(g.urlPatch(changeNum, revisionNum))
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to patch")
	}

	err = helper(path, proto.Base64Patch, string(buf))
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to fetch")
	}

	// Get content
	fs := current["files"].(map[string]interface{})

	for key := range fs {
		buf, err := g.get(g.urlContent(changeNum, revisionNum, key))
		if err != nil {
			return "", nil, errors.Wrap(err, "failed to content")
		}

		err = helper(filepath.Join(path, filepath.Dir(key)), filepath.Base(key)+proto.Base64Content, string(buf))
		if err != nil {
			return "", nil, errors.Wrap(err, "failed to fetch")
		}
	}

	var files []string

	files = append(files, proto.Base64Patch)
	for key := range fs {
		files = append(files, key)
	}

	return root, files, nil
}

func (g *gerrit) Vote(commit string, data []proto.Format) error {
	helper := func() (map[string]interface{}, map[string]interface{}, string) {
		if len(data) == 0 {
			return nil, map[string]interface{}{g.r.Vote.Label: g.r.Vote.Approval}, g.r.Vote.Message
		}
		c := map[string]interface{}{}
		for _, item := range data {
			b := map[string]interface{}{"line": item.Line, "message": item.Details}
			if _, p := c[item.File]; !p {
				c[item.File] = []map[string]interface{}{b}
			} else {
				c[item.File] = append(c[item.File].([]map[string]interface{}), b)
			}
		}
		return c, map[string]interface{}{g.r.Vote.Label: g.r.Vote.Disapproval}, g.r.Vote.Message
	}

	r, err := g.get(g.urlQuery("commit:"+commit, []string{"CURRENT_REVISION"}, 0))
	if err != nil {
		return errors.Wrap(err, "failed to query")
	}

	ret, err := g.unmarshalList(r)
	if err != nil {
		return errors.Wrap(err, "failed to unmarshal")
	}

	revisions := ret["revisions"].(map[string]interface{})
	current := revisions[ret["current_revision"].(string)].(map[string]interface{})

	comments, labels, message := helper()
	buf := map[string]interface{}{"comments": comments, "labels": labels, "message": message}

	if err := g.post(g.urlReview(int(ret["_number"].(float64)), int(current["_number"].(float64))), buf); err != nil {
		return errors.Wrap(err, "failed to review")
	}

	return nil
}

func (g *gerrit) unmarshal(data []byte) (map[string]interface{}, error) {
	buf := map[string]interface{}{}

	if err := json.Unmarshal(data[4:], &buf); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal")
	}

	return buf, nil
}

func (g *gerrit) unmarshalList(data []byte) (map[string]interface{}, error) {
	var buf []map[string]interface{}

	if err := json.Unmarshal(data[4:], &buf); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal")
	}

	if len(buf) == 0 {
		return nil, errors.New("failed to match")
	}

	return buf[0], nil
}

func (g *gerrit) urlContent(change, revision int, name string) string {
	buf := g.r.Host + ":" + strconv.Itoa(g.r.Port) + "/changes/" + strconv.Itoa(change) +
		"/revisions/" + strconv.Itoa(revision) + "/files/" + url.PathEscape(name) + "/content"

	if g.r.User != "" && g.r.Pass != "" {
		buf = g.r.Host + ":" + strconv.Itoa(g.r.Port) + "/a/changes/" + strconv.Itoa(change) +
			"/revisions/" + strconv.Itoa(revision) + "/files/" + url.PathEscape(name) + "/content"
	}

	return buf
}

func (g *gerrit) urlDetail(change int) string {
	buf := g.r.Host + ":" + strconv.Itoa(g.r.Port) + "/changes/" + strconv.Itoa(change) + "/detail"

	if g.r.User != "" && g.r.Pass != "" {
		buf = g.r.Host + ":" + strconv.Itoa(g.r.Port) + "/a/changes/" + strconv.Itoa(change) + "/detail"
	}

	return buf
}

func (g *gerrit) urlPatch(change, revision int) string {
	buf := g.r.Host + ":" + strconv.Itoa(g.r.Port) + "/changes/" + strconv.Itoa(change) +
		"/revisions/" + strconv.Itoa(revision) + "/patch"

	if g.r.User != "" && g.r.Pass != "" {
		buf = g.r.Host + ":" + strconv.Itoa(g.r.Port) + "/a/changes/" + strconv.Itoa(change) +
			"/revisions/" + strconv.Itoa(revision) + "/patch"
	}

	return buf
}

func (g *gerrit) urlQuery(search string, option []string, start int) string {
	query := "?q=" + search + "&o=" + strings.Join(option, "&o=") + "&n=" + strconv.Itoa(start)

	buf := g.r.Host + ":" + strconv.Itoa(g.r.Port) + "/changes/" + query
	if g.r.User != "" && g.r.Pass != "" {
		buf = g.r.Host + ":" + strconv.Itoa(g.r.Port) + "/a/changes/" + query
	}

	return buf
}

func (g *gerrit) urlReview(change, revision int) string {
	buf := g.r.Host + ":" + strconv.Itoa(g.r.Port) + "/changes/" + strconv.Itoa(change) +
		"/revisions/" + strconv.Itoa(revision) + "/review"

	if g.r.User != "" && g.r.Pass != "" {
		buf = g.r.Host + ":" + strconv.Itoa(g.r.Port) + "/a/changes/" + strconv.Itoa(change) +
			"/revisions/" + strconv.Itoa(revision) + "/review"
	}

	return buf
}

func (g *gerrit) get(_url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, _url, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to request")
	}

	if g.r.User != "" && g.r.Pass != "" {
		req.SetBasicAuth(g.r.User, g.r.Pass)
	}

	rsp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to do")
	}

	defer func() {
		_ = rsp.Body.Close()
	}()

	if rsp.StatusCode != http.StatusOK {
		return nil, errors.New("invalid status")
	}

	data, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read")
	}

	return data, nil
}

func (g *gerrit) post(_url string, data map[string]interface{}) error {
	buf, err := json.Marshal(data)
	if err != nil {
		return errors.Wrap(err, "failed to marshal")
	}

	req, err := http.NewRequest(http.MethodPost, _url, bytes.NewBuffer(buf))
	if err != nil {
		return errors.Wrap(err, "failed to request")
	}

	req.Header.Set("Content-Type", "application/json;charset=utf-8")

	if g.r.User != "" && g.r.Pass != "" {
		req.SetBasicAuth(g.r.User, g.r.Pass)
	}

	rsp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "failed to do")
	}

	defer func() {
		_ = rsp.Body.Close()
	}()

	if rsp.StatusCode != http.StatusOK {
		return errors.New("invalid status")
	}

	_, err = ioutil.ReadAll(rsp.Body)
	if err != nil {
		return errors.Wrap(err, "failed to read")
	}

	return nil
}
