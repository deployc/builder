package main

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

const Python3Dockerfile = `FROM python:3.6-alpine
WORKDIR /app
COPY . ./
RUN pip install -r requirements.txt
CMD ["python", "main.py"]`

const JavascriptDockerfile = `FROM node:10-alpine
WORKDIR /app
COPY . ./
RUN npm install
CMD ["npm", "start"]`

const GoDockerfile = `FROM golang:1.10-alpine
WORKDIR /go/src/app
COPY . ./
RUN go get -v ./...
RUN go install
CMD ["/go/bin/app"]`

type Detector struct {
	dir         string
	dockerfiles map[string]string
}

func NewDetector(dir string) Detector {
	return Detector{dir: dir, dockerfiles: map[string]string{}}
}

func fileExists(f string) bool {
	_, err := os.Stat(f)
	return !os.IsNotExist(err)
}

func readFile(f string) string {
	df, err := ioutil.ReadFile(f)
	if err == nil || err == io.EOF {
		return string(df)
	}
	return ""
}

func (d Detector) checkType(idFile, ty, dockerfile string) {
	f := filepath.Join(d.dir, idFile)
	if fileExists(f) {
		d.dockerfiles[ty] = dockerfile
	}
}

func (d Detector) detectType() (string, string, error) {
	f := filepath.Join(d.dir, "Dockerfile")
	if fileExists(f) {
		// Dockerfile trumps all
		return "Dockerfile", readFile(f), nil
	}

	d.checkType("package.json", "JavaScript", JavascriptDockerfile)
	d.checkType("requirements.txt", "Python", Python3Dockerfile)
	d.checkType("main.go", "Go", GoDockerfile)

	l := len(d.dockerfiles)
	if l == 0 {
		return "", "", errors.New("Could not determine build type.")
	}

	if l > 1 {
		possible := []string{}
		for ty := range d.dockerfiles {
			possible = append(possible, ty)
		}
		err := fmt.Sprintf("Could not determine build type. Found multiple types: %s", strings.Join(possible, ", "))
		return "", "", errors.New(err)
	}

	for ty, f := range d.dockerfiles {
		return ty, f, nil
	}

	return "", "", nil
}
