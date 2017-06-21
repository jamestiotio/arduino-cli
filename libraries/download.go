/*
 * This file is part of arduino-cli.
 *
 * arduino-cli is free software; you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation; either version 2 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program; if not, write to the Free Software
 * Foundation, Inc., 51 Franklin St, Fifth Floor, Boston, MA  02110-1301  USA
 *
 * As a special exception, you may use this file as part of a free software
 * library without restriction.  Specifically, if other files instantiate
 * templates or use macros or inline functions from this file, or you compile
 * this file and link it with other files to produce an executable, this
 * file does not by itself cause the resulting executable to be covered by
 * the GNU General Public License.  This exception does not however
 * invalidate any other reasons why the executable file might be covered by
 * the GNU General Public License.
 *
 * Copyright 2017 BCMI LABS SA (http://www.arduino.cc/)
 */

package libraries

import (
	"archive/zip"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"

	"github.com/bcmi-labs/arduino-cli/common"
)

const (
	libraryIndexURL string = "http://downloads.arduino.cc/libraries/library_index.json"
)

// DownloadAndCache downloads a library without installing it
func DownloadAndCache(library *Library, progressFiles int, totalFiles int) (*zip.Reader, error) {
	zipContent, err := downloadLatest(library, progressFiles, totalFiles)
	if err != nil {
		return nil, err
	}

	zipArchive, err := prepareInstall(library, zipContent)
	if err != nil {
		return nil, err
	}

	return zipArchive, nil
}

// DownloadLatest downloads Latest version of a library.
func downloadLatest(library *Library, progressFiles int, totalFiles int) ([]byte, error) {
	return common.DownloadPackage(library.Latest().URL, fmt.Sprintf("library %s", library.Name), progressFiles, totalFiles)
}

// DownloadLibrariesFile downloads the lib file from arduino repository.
func DownloadLibrariesFile() error {
	libFile, err := IndexPath()
	if err != nil {
		return err
	}

	req, err := http.NewRequest("GET", libraryIndexURL, nil)
	if err != nil {
		return err
	}

	client := http.DefaultClient
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(libFile, content, 0666)
	if err != nil {
		return err
	}
	return nil
}

// getDownloadCacheFolder gets the folder where temp installs are stored until installation complete (libraries).
func getDownloadCacheFolder() (string, error) {
	libFolder, err := common.GetDefaultArduinoFolder()
	if err != nil {
		return "", err
	}

	stagingFolder := filepath.Join(libFolder, "staging")
	return common.GetFolder(stagingFolder, "libraries cache")
}