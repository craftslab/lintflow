#!/bin/bash

docker pull craftslab/gerritdocker:test-lintflow
docker run --rm -p 8080:8080 -p 29418:29418 craftslab/gerritdocker:test-lintflow run
