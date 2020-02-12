mkdir -p /home/runner/src/github.com/containous
cp -r /home/runner/maesh /home/runner/src/github.com/containous
cd /home/runner/src/github.com/containous/maesh
if [ -f "./go.mod" ]; then export GO111MODULE=on; fi
if [ -f "./go.mod" ]; then export GOPROXY=https://proxy.golang.org; fi
if [ -f "./go.mod" ]; then go mod download; fi
df
