// +build window

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/gonutz/w32"
	"github.com/shurcooL/httpfs/html/vfstemplate"
)

const maxUploadSize = 10 * 1024 * 1024 * 1024 // 10 gb
const uploadPath = "uploads"

func main() {
	// Get current IP address
	var currentIP = "localhost"
	var addrs, err = net.InterfaceAddrs()
	if err != nil {
		log.Println("Cannot get server IP")
		log.Println(err)
	}

	for _, address := range addrs {
		// check the address type and if it is not a loopback the display it
		// = GET LOCAL IP ADDRESS
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				currentIP = ipnet.IP.String()
			}
		}
	}

	// Usage doc
	// fmt.Printf("Usage: file_server.exe -path=\"file_path\" -port=port_number\n")
	// fmt.Printf("Default value: -path=\"%s\" -port=9000\n\n", homePath)

	// Set default port, path and hide console window option
	hide := "N"
	hideAfter := 5
	port := 9000
	path, err := os.Getwd()
	if err != nil {
		log.Println(err)
	}

	// Get arguments
	fpath := flag.String("path", "N/A", "home path")
	fport := flag.Int("port", -1, "web server port")
	fhide := flag.String("hide", "N/A", "hide console window")
	fdelay := flag.Int("delay", -1, "hide console windows after (seconds)")
	flag.Parse()

	// If there is no arguments, request user input
	if *fpath != "N/A" {
		path = *fpath
	} else {
		fmt.Printf("Folder to serve? (%s): ", path)
		fmt.Scanln(&path)

		if _, err := os.Stat(path); os.IsNotExist(err) {
			log.Fatal(err)
		}
	}

	if *fport != -1 {
		port = *fport
	} else {
		fmt.Printf("Port to serve? (%d): ", port)
		fmt.Scanln(&port)

		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			log.Fatal(err)
		}
		ln.Close()
	}

	if runtime.GOOS == "windows" {
		if *fhide != "N/A" {
			hide = *fhide
		} else {
			fmt.Print("Do you want to hide this window after File Server started (Y/N)? ")
			fmt.Scanln(&hide)
		}

		if hide == "Y" {
			if *fdelay != -1 {
				delay = *fdelay
			} else {
				fmt.Print("This console window will be hide after (seconds)? ")
				fmt.Scanln(&hideAfter)
			}
		}
	}

	fmt.Printf("Serve file in: %s\n", path)
	fmt.Printf("File Server will be started on: http://%s:%d after some seconds\n", currentIP, port)
	fmt.Printf("You can upload files to server on: http://%s:%d/upload\n", currentIP, port)

	fullUploadPath := filepath.Join(path, uploadPath)

	fs := http.FileServer(http.Dir(path))
	http.Handle("/", http.StripPrefix("", fs))
	// http.Handle("/files/", http.StripPrefix("/files", fs))
	http.HandleFunc("/upload", uploadFileHandler(fullUploadPath))

	if runtime.GOOS == "windows" && hide == "Y" && delay >= 0 {
		time.Sleep(delay * 1000)
		hideConsole()
	}

	err = http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
	if err != nil {
		log.Println(err)
		showConsole()
		pauseProcess()
	}
}

func uploadFileHandler(uploadPath string) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			// t, _ := template.ParseFiles("upload.gtpl")
			t, err := vfstemplate.ParseFiles(assets, nil, "upload.gtpl")
			if err != nil {
				fmt.Println(err)
			}
			t.Execute(w, nil)
			return
		}

		if err := r.ParseMultipartForm(maxUploadSize); err != nil {
			fmt.Printf("Could not parse multipart form: %v\n", err)
			renderError(w, "CANT_PARSE_FORM", http.StatusInternalServerError)
			return
		}

		// parse and validate file and post parameters
		file, fileHeader, err := r.FormFile("uploadFile")
		if err != nil {
			renderError(w, "INVALID_FILE", http.StatusBadRequest)
			return
		}

		defer file.Close()
		// Get and print out file size
		fileSize := fileHeader.Size
		fmt.Printf("File size (bytes): %v\n", fileSize)
		// Validate file size
		if fileSize > maxUploadSize {
			renderError(w, "FILE_TOO_BIG", http.StatusBadRequest)
			return
		}
		fileBytes, err := ioutil.ReadAll(file)
		if err != nil {
			renderError(w, "INVALID_FILE", http.StatusBadRequest)
			return
		}

		// Check file type, detect content type only needs the first 512 bytes
		detectedFileType := http.DetectContentType(fileBytes)
		switch detectedFileType {
		case "img/jpeg", "image/jpg":
		case "image/gif", "image/png":
		case "application/pdf":
			break
		default:
			// Allow all file tye
			break
			// renderError(w, "INVALID_FILE_TYPE", http.StatusBadRequest)
			// return
		}

		fileName := fileHeader.Filename
		_, err = mime.ExtensionsByType(detectedFileType)
		if err != nil {
			renderError(w, "CANT_READ_FILE_TYPE", http.StatusInternalServerError)
			return
		}

		newPath := filepath.Join(uploadPath, fileName)
		fmt.Printf("FileType: %s, File: %s\n", detectedFileType, newPath)

		// Write file
		err = os.MkdirAll(filepath.Dir(newPath), 0770)
		if err != nil {
			renderError(w, "CANT_WRITE_FILE", http.StatusInternalServerError)
			return
		}

		newFile, err := os.Create(newPath)
		if err != nil {
			fmt.Println(err)
			renderError(w, "CANT_WRITE_FILE", http.StatusInternalServerError)
			return
		}

		defer newFile.Close()
		if _, err := newFile.Write(fileBytes); err != nil && newFile.Close() != nil {
			renderError(w, "CANT_WRITE_FILE", http.StatusInternalServerError)
			return
		}
		w.Write([]byte("SUCCESS"))
	})
}

// Pause program and wait user press Enter to exit
func pauseProcess() {
	fmt.Printf("\nPress Enter Key to exit...")
	fmt.Scanln()
}

func renderError(w http.ResponseWriter, message string, statusCode int) {
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte(message))
}

func hideConsole() {
	console := w32.GetConsoleWindow()
	if console == 0 {
		return // no console attached
	}
	// If this application is the process that created the console window, then
	// this program was not compiled with the -H=windowsgui flag and on start-up
	// it created a console along with the main application window. In this case
	// hide the console window.
	// See
	// http://stackoverflow.com/questions/9009333/how-to-check-if-the-program-is-run-from-a-console
	_, consoleProcID := w32.GetWindowThreadProcessId(console)
	if w32.GetCurrentProcessId() == consoleProcID {
		w32.ShowWindowAsync(console, w32.SW_HIDE)
	}
}

func showConsole() {
	console := w32.GetConsoleWindow()
	if console == 0 {
		return // no console attached
	}

	_, consoleProcID := w32.GetWindowThreadProcessId(console)
	if w32.GetCurrentProcessId() == consoleProcID {
		w32.ShowWindowAsync(console, w32.SW_SHOW)
	}
}
