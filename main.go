package main

import (
	"archive/tar"
	"bytes"
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/satori/go.uuid"
)

func extractTar(dir string, tr *tar.Reader) error {
	// Extracting tarred files
	for {
		header, err := tr.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Println(err)
			return err
		}

		// get the individual filename and extract to the current directory
		filename := header.Name

		switch header.Typeflag {
		case tar.TypeDir:
			// handle directory
			path := filepath.Join(dir, filename)
			fmt.Printf("Creating directory: %s\n", path)
			err = os.MkdirAll(path, os.FileMode(header.Mode)) // or use 0755 if you prefer
			if err != nil {
				fmt.Println(err)
				return err
			}
		case tar.TypeReg:
			// handle normal file
			path := filepath.Join(dir, filename)
			fmt.Printf("Untarring: %s\n", path)
			writer, err := os.Create(path)
			if err != nil {
				fmt.Println(err)
				return err
			}

			io.Copy(writer, tr)
			err = os.Chmod(path, os.FileMode(header.Mode))
			if err != nil {
				fmt.Println(err)
				return err
			}

			writer.Close()
		default:
			fmt.Printf("Unable to untar type : %c in file %s", header.Typeflag, filename)
		}
	}

	return nil
}

func prefixingWriter(prefix string, tag string, output io.Writer) *io.PipeWriter {
	pipeReader, pipeWriter := io.Pipe()
	scanner := bufio.NewScanner(pipeReader)
	scanner.Split(bufio.ScanLines)

	go func() {
		defer pipeReader.Close()
		for scanner.Scan() {
			fmt.Fprintf(output, "[%s]", prefix)
			b := scanner.Bytes()
			b = bytes.Replace(b, []byte(tag + ":latest"), []byte("image"), -1)
			b = bytes.Replace(b, []byte(tag), []byte("image"), -1)
			output.Write(b)
			fmt.Fprintf(output, "\n")
		}
	}()
	return pipeWriter
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	// Get image tag used to build and push to registry
	u, err := uuid.NewV4()
	if err != nil {
		fmt.Fprintf(conn, "[result]FAILED\n")
		return
	}
	tag := fmt.Sprintf("registry.deployc.svc.cluster.local/%s", u.String())

	// Create writers
	stdout := prefixingWriter("stdout", tag, conn)
	stderr := prefixingWriter("stderr", tag, conn)
	defer stdout.Close()
	defer stderr.Close()

	// Get temp directory
	dir, err := ioutil.TempDir("", "build-")
	if err != nil {
		fmt.Fprintf(stderr, "%s\n", err.Error())
		fmt.Fprintf(conn, "[result]FAILED\n")
		return
	}

	// Extract the tar file
	tr := tar.NewReader(conn)
	if err := extractTar(dir, tr); err != nil {
		fmt.Fprintf(stderr, "%s\n", err.Error())
		fmt.Fprintf(conn, "[result]FAILED\n")
		return
	}

	// Detect type
	detector := NewDetector(dir)
	ty, df, err := detector.detectType()
	if err != nil {
		fmt.Fprintf(stderr, "%s\n", err.Error())
		fmt.Fprintf(conn, "[result]FAILED\n")
		return
	}

	// Write Dockerfile out, if it doesn't exist
	dfPath := filepath.Join(dir, "Dockerfile")
	if !fileExists(dfPath) {
		f, err := os.Create(dfPath)
		if err != nil {
			fmt.Fprintf(stderr, "%s\n", err.Error())
			fmt.Fprintf(conn, "[result]FAILED\n")
			return
		}

		defer f.Close()
		f.WriteString(df)
		f.Close()
	}

	fmt.Fprintf(stdout, "\nFound %s app. Building.\n\n", ty)

	// Run the build command
	cmd := exec.Command("img", "build", "-t", tag, dir)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err = cmd.Run(); err != nil {
		fmt.Fprintf(stderr, "%s\n", err.Error())
		fmt.Fprintf(conn, "[result]FAILED\n")
		return
	}

	// Now, push to the registry
	cmd = exec.Command("img", "push", tag)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err = cmd.Run(); err != nil {
		fmt.Fprintf(stderr, "%s\n", err.Error())
		fmt.Fprintf(conn, "[result]FAILED\n")
		return
	}

	// Write the image tag name as the last thing
	fmt.Fprintf(stdout, "%s", "\n")
	fmt.Fprintf(conn, "[result]%s\n", tag)

	// Clean up
	err = os.RemoveAll(dir)
	if err != nil {
		fmt.Printf("Could not clean up: %s\n", err.Error())
		return
	}
}

func main() {
	// Bind to port 9393
	l, err := net.Listen("tcp", "[::]:9393")
	if err != nil {
		fmt.Printf("Could not listen: %s\n", err.Error())
		os.Exit(1)
	}

	// Close after we exit
	defer l.Close()

	// Listen for incoming connections
	fmt.Println("Listening on [::]:9393")
	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Printf("Could not accept client: %s\n", err.Error())
			os.Exit(1)
		}

		go handleConnection(conn)
	}
}
