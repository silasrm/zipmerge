// Package main provides a graphical user interface for selecting, unzipping, and merging ZIP files.
// The program utilizes fyne.io for creating the GUI and zenity for file/folder dialogs.
// The main purpose of this tool is to allow users to simplify ZIP file management by combining multiple files.

package main

import (
	"archive/zip"
	"fmt"
	"image/color"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/ncruces/zenity"
)

// setFiles assigns the value of a string slice to a pointer.
// This helper function is used to update lists of files selected by the user during execution.
func setFiles(ptr *[]string, value []string) {
	*ptr = value
}

// ZipFiles compresses a source directory or file into a destination ZIP file.
// Parameters:
// - source: Path to the directory or single file that will be zipped.
// - destination: Path where the output ZIP file will be created.
// Returns an error if there are issues creating/writing the ZIP file.
func ZipFiles(source, destination string) error {
	outFile, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer outFile.Close()

	zipWriter := zip.NewWriter(outFile)
	defer zipWriter.Close() //

	// Use filepath.Walk to add files and directories to the zip
	err = filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get the relative path of the file/dir to the source
		relPath, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relPath == "." { // Skip the source directory itself
			return nil
		}

		// Create a header for the file
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = relPath

		if info.IsDir() {
			header.Name += "/" // Add trailing slash for directories
		} else {
			// Set the compression method
			header.Method = zip.Deflate
		}

		headerWriter, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}

		if !info.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()
			_, err = io.Copy(headerWriter, file) // Copy file content into the zip
			if err != nil {
				return err
			}
		}

		return nil
	})

	return err
}

// Unzip extracts the contents of a ZIP file to a destination directory.
// Parameters:
// - src: Path to the source ZIP file to be extracted.
// - dest: Path to the destination directory where files will be unpacked.
// Returns a slice of extracted file names and an error if any issues occur.
func Unzip(src, dest string) ([]string, error) {
	files := []string{}

	reader, err := zip.OpenReader(src) //
	if err != nil {
		return files, err
	}
	defer reader.Close()

	for _, file := range reader.File {
		filename := filepath.Base(file.Name)
		filePath := filepath.Join(dest, filename)
		log.Println("Unzipping", filePath)
		log.Println(">", file.Name)
		log.Println(">", filename)

		// Check for Zip Slip vulnerability
		if !strings.HasPrefix(filePath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return files, fmt.Errorf("invalid file path: %s", filePath) //
		}

		if file.FileInfo().IsDir() {
			os.MkdirAll(filePath, os.ModePerm)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
			return files, err
		}

		outFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			return files, err
		}

		rc, err := file.Open()
		if err != nil {
			outFile.Close()
			return files, err
		}

		_, err = io.Copy(outFile, rc) // Copy file content out of the zip

		// Close the file writers and readers immediately to avoid resource issues
		outFile.Close()
		rc.Close()

		if err != nil {
			return files, err
		}
	}

	return files, nil
}

func main() {
	// Initialize state variables and UI components.
	multiple := []string{}                           // Stores the list of selected ZIP files.
	targetFolderBinding := binding.NewString()       // Data binding for tracking the selected target folder.
	a := app.New()                                   // Create the application instance.
	w := a.NewWindow("Zip Merge")                    // Create the main application window.
	green := color.NRGBA{R: 0, G: 180, B: 0, A: 255} // Define text color for the header.
	progress := widget.NewProgressBar()              // Progress bar for visualizing the zipping/unzipping process.

	targetFolderBinding.Set("Nenhuma pasta selecionada.")

	w.SetMaster()
	w.Resize(fyne.NewSize(400, 300))

	text := canvas.NewText("Selecione os arquivos ZIP que deverão ser combinados.", green)
	text.Alignment = fyne.TextAlignLeading
	text.TextStyle = fyne.TextStyle{Italic: true, Bold: true}

	// Create a button to select multiple ZIP files.
	btnSelectFiles := widget.NewButton("Selecionar arquivo(s)", func() {
		files, err := zenity.SelectFileMultiple(
			zenity.Filename(``), // Default filename or directory path.
			zenity.FileFilters{
				{"ZIP files", []string{"*.zip"}, false}, // Limit file selection to ZIP files.
			})

		if err != nil {
			return // Exit if the user cancels or an error occurs.
		}

		setFiles(&multiple, files) // Update the list of selected files.

		log.Println("Arquivo selecionado: ", multiple) // Log the selected files for debugging.
	})

	btntargetFolderBinding := widget.NewButton("Selecionar pasta de destino", func() {
		// Open a directory selection dialog
		folderPath, err := zenity.SelectFile(
			zenity.Directory(),              // Option to select a directory instead of a file
			zenity.Title("Select a Folder"), // Set the dialog title
		)

		if err != nil {
			// Handle errors, including the user canceling the dialog
			if err == zenity.ErrCanceled {
				log.Println("Dialog canceled by the user")
				return
			}
			log.Fatal(err)
		}

		targetFolderBinding.Set(folderPath)
		log.Printf("Selected folder: %s\n", folderPath)
	})

	// Create a button to start the zipping/unzipping process.
	btnProcess := widget.NewButton("Processar", func() {
		go func() {
			// Get the selected target folder path from the binding.
			targetFolder, err := targetFolderBinding.Get()
			if err != nil {
				// Handle error if necessary
				log.Println("Error getting value:", err)
				return
			}

			tmpDir := fmt.Sprintf("%s/%s", targetFolder, "tmp")
			finalZip := fmt.Sprintf("%s/%s", targetFolder, "all.zip")
			listFilesExtracteds := []string{}

			log.Println("Target folder: ", tmpDir)

			// Create a dummy directory and files for demonstration
			os.Mkdir(tmpDir, os.ModePerm)

			// 1. Extract all files
			//zipFile := "./archive.zip"
			//destDir := "./extracted_files"
			i := 0
			for _, zipFile := range multiple {
				log.Println("Extracting file: ", zipFile)
				if files, err := Unzip(zipFile, tmpDir); err != nil {
					log.Fatal("Error unzipping file:", err)
				} else {
					log.Println("Successfully extracted: ", files)
					listFilesExtracteds = append(listFilesExtracteds, zipFile)
				}

				time.Sleep(time.Millisecond * 250)
				progress.SetValue(float64(i))
				i += 1
			}

			log.Println("Successfully unzipped:", listFilesExtracteds)

			// 2. Compact all files from this folder
			if err := ZipFiles(tmpDir, finalZip); err != nil {
				log.Fatal(err)
			}
			log.Printf("Successfully zipped %s to %s\n", tmpDir, finalZip)

			time.Sleep(time.Millisecond * 250)
			progress.SetValue(float64(i))
		}()
	})

	list := widget.NewList(
		func() int {
			return len(multiple)
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("template")
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			o.(*widget.Label).SetText(multiple[i])
		})

	folderText := widget.NewLabelWithData(targetFolderBinding)
	folderText.Alignment = fyne.TextAlignLeading
	folderText.TextStyle = fyne.TextStyle{Italic: true, Bold: true}

	entry := widget.NewEntry()
	entry.Bind(targetFolderBinding)

	grid := container.New(
		layout.NewGridLayoutWithRows(4),
		container.NewGridWithRows(2, text, btnSelectFiles),
		container.New(
			layout.NewPaddedLayout(),
			container.NewPadded(list),
		),
		container.NewVBox(btntargetFolderBinding, folderText),
		container.NewVBox(btnProcess, progress),
	)

	w.SetContent(grid)
	w.ShowAndRun()
}
