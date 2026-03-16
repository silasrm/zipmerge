package main

import (
	"archive/zip"
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/ncruces/zenity"
	"image/color"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func setFiles(ptr *[]string, value []string) {
	*ptr = value
}

// ZipFiles compresses a source directory/file into a destination zip file.
func ZipFiles(source, destination string) error {
	outFile, err := os.Create(destination) //
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

// Unzip extracts a zip file to a destination directory.
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
	multiple := []string{}
	targetFolderBinding := binding.NewString()
	a := app.New()
	w := a.NewWindow("Zip Merge")
	green := color.NRGBA{R: 0, G: 180, B: 0, A: 255}
	progress := widget.NewProgressBar()

	targetFolderBinding.Set("Nenhuma pasta selecionada.")

	w.SetMaster()
	w.Resize(fyne.NewSize(400, 300))

	text := canvas.NewText("Selecione os arquivos ZIP que deverão ser combinados.", green)
	text.Alignment = fyne.TextAlignLeading
	text.TextStyle = fyne.TextStyle{Italic: true, Bold: true}

	btnSelectFiles := widget.NewButton("Selecionar arquivo(s)", func() {
		files, err := zenity.SelectFileMultiple(
			zenity.Filename(``),
			zenity.FileFilters{
				{"ZIP files", []string{"*.zip"}, false},
			})

		if err != nil {
			return
		}

		setFiles(&multiple, files)

		log.Println("Arquivo selecionado: ", multiple)
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

	btnProcess := widget.NewButton("Processar", func() {
		go func() {
			// Get the value from the binding
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
