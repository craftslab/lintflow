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

package config

type Config struct {
	ApiVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	MetaData   MetaData `yaml:"metadata"`
	Spec       Spec     `yaml:"spec"`
}

type MetaData struct {
	Name string `yaml:"name"`
}

type Spec struct {
	Lint   []Lint   `yaml:"lint"`
	Review []Review `yaml:"review"`
}

type Lint struct {
	Filter  Filter `yaml:"filter"`
	Host    string `yaml:"host"`
	Name    string `yaml:"name"`
	Port    int    `yaml:"port"`
	Timeout int    `yaml:"timeout"`
}

type Filter struct {
	Include Include `yaml:"include"`
}

type Include struct {
	Extension []string `yaml:"extension"`
	File      []string `yaml:"file"`
	Repo      []string `yaml:"repo"`
}

type Review struct {
	Host string `yaml:"host"`
	Name string `yaml:"name"`
	Pass string `yaml:"pass"`
	Port int    `yaml:"port"`
	User string `yaml:"user"`
	Vote Vote   `yaml:"vote"`
}

type Vote struct {
	Approval    string `yaml:"approval"`
	Disapproval string `yaml:"disapproval"`
	Label       string `yaml:"label"`
	Message     string `yaml:"message"`
}

var (
	Build   string
	Version string
)

func New() *Config {
	return &Config{}
}
