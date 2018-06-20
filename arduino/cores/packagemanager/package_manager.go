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
 * Copyright 2017-2018 ARDUINO AG (http://www.arduino.cc/)
 */

package packagemanager

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	properties "github.com/arduino/go-properties-map"
	"github.com/bcmi-labs/arduino-cli/arduino/cores"
	"github.com/bcmi-labs/arduino-cli/arduino/cores/packageindex"
	"github.com/bcmi-labs/arduino-cli/configs"
	"github.com/sirupsen/logrus"
)

// PackageManager defines the superior oracle which understands all about
// Arduino Packages, how to parse them, download, and so on.
//
// The manager also keeps track of the status of the Packages (their Platform Releases, actually)
// installed in the system.
type PackageManager struct {
	Log      logrus.FieldLogger
	packages *cores.Packages

	// TODO: This might be a list in the future, but would it be of any help?
	eventHandler EventHandler
}

// EventHandler defines the events that are generated by the PackageManager
// Subscribing to such events allows, for instance, to print out logs of what is happening
// (say you use them for a CLI...)
type EventHandler interface {
	// FIXME: This is temporary, for prototyping (an handler should not return an handler; besides, this leakes
	// the usage of releases...)
}

// NewPackageManager returns a new instance of the PackageManager
func NewPackageManager() *PackageManager {
	return &PackageManager{
		Log:      logrus.StandardLogger(),
		packages: cores.NewPackages(),
	}
}

func (pm *PackageManager) Clear() {
	pm.packages = cores.NewPackages()
}

func (pm *PackageManager) GetPackages() *cores.Packages {
	return pm.packages
}

func (pm *PackageManager) FindBoardsWithVidPid(vid, pid string) []*cores.Board {
	res := []*cores.Board{}
	for _, targetPackage := range pm.packages.Packages {
		for _, targetPlatform := range targetPackage.Platforms {
			if platform := targetPlatform.GetInstalled(); platform != nil {
				for _, board := range platform.Boards {
					if board.HasUsbID(vid, pid) {
						res = append(res, board)
					}
				}
			}
		}
	}
	return res
}

func (pm *PackageManager) FindBoardsWithID(id string) []*cores.Board {
	res := []*cores.Board{}
	for _, targetPackage := range pm.packages.Packages {
		for _, targetPlatform := range targetPackage.Platforms {
			if platform := targetPlatform.GetInstalled(); platform != nil {
				for _, board := range platform.Boards {
					if board.BoardId == id {
						res = append(res, board)
					}
				}
			}
		}
	}
	return res
}

// FindBoardWithFQBN returns the board identified by the fqbn, or an error
func (pm *PackageManager) FindBoardWithFQBN(fqbnIn string) (*cores.Board, error) {
	fqbn, err := cores.ParseFQBN(fqbnIn)
	if err != nil {
		return nil, fmt.Errorf("parsing fqbn: %s", err)
	}

	_, _, board, _, _, err := pm.ResolveFQBN(fqbn)
	return board, err
}

// ResolveFQBN returns, in order:
// - the Package pointed by the fqbn
// - the PlatformRelease pointed by the fqbn
// - the Board pointed by the fqbn
// - the build properties for the board considering also the
//   configuration part of the fqbn
// - the PlatformRelease to be used for the build (if the board
//   requires a 3rd party core it may be different from the
//   PlatformRelease pointed by the fqbn)
// - an error if any of the above is not found
//
// In case of error the partial results found in the meantime are
// returned together with the error.
func (pm *PackageManager) ResolveFQBN(fqbn *cores.FQBN) (
	*cores.Package, *cores.PlatformRelease, *cores.Board,
	properties.Map, *cores.PlatformRelease, error) {

	// Find package
	targetPackage := pm.packages.Packages[fqbn.Package]
	if targetPackage == nil {
		return nil, nil, nil, nil, nil,
			errors.New("unknown package " + fqbn.Package)
	}

	// Find platform
	platform := targetPackage.Platforms[fqbn.PlatformArch]
	if platform == nil {
		return targetPackage, nil, nil, nil, nil,
			fmt.Errorf("unknown platform %s:%s", targetPackage, fqbn.PlatformArch)
	}
	platformRelease := platform.GetInstalled()
	if platformRelease == nil {
		return targetPackage, nil, nil, nil, nil,
			fmt.Errorf("Platform %s is not installed", platformRelease)
	}

	// Find board
	board := platformRelease.Boards[fqbn.BoardID]
	if board == nil {
		return targetPackage, platformRelease, nil, nil, nil,
			fmt.Errorf("board %s:%s not found", platformRelease, fqbn.BoardID)
	}

	buildProperties, err := board.GetBuildProperties(fqbn.Configs)
	if err != nil {
		return targetPackage, platformRelease, board, nil, nil,
			fmt.Errorf("getting build properties for board %s: %s", board, err)
	}

	// Determine the platform used for the build (in case the board refers
	// to a core contained in another platform)
	buildPlatformRelease := platformRelease
	coreParts := strings.Split(buildProperties["build.core"], ":")
	if len(coreParts) > 1 {
		referredPackage := coreParts[1]
		buildPackage := pm.packages.Packages[referredPackage]
		if buildPackage == nil {
			return targetPackage, platformRelease, board, buildProperties, nil,
				fmt.Errorf("missing package %s:%s required for build", referredPackage, platform)
		}
		buildPlatformRelease = buildPackage.Platforms[fqbn.PlatformArch].GetInstalled()
	}

	// No errors... phew!
	return targetPackage, platformRelease, board, buildProperties, buildPlatformRelease, nil
}

// FIXME add an handler to be invoked on each verbose operation, in order to let commands display results through the formatter
// as for the progress bars during download
func (pm *PackageManager) RegisterEventHandler(eventHandler EventHandler) {
	if pm.eventHandler != nil {
		panic("Don't try to register another event handler to the PackageManager yet!")
	}

	pm.eventHandler = eventHandler
}

// GetEventHandlers returns a slice of the registered EventHandlers
func (pm *PackageManager) GetEventHandlers() []*EventHandler {
	return append([]*EventHandler{}, &pm.eventHandler)
}

// LoadPackageIndex loads a package index by looking up the local cached file from the specified URL
func (pm *PackageManager) LoadPackageIndex(URL *url.URL) error {
	indexPath, err := configs.IndexPathFromURL(URL).Get()
	if err != nil {
		return fmt.Errorf("retrieving json index path for %s: %s", URL, err)
	}

	index, err := packageindex.LoadIndex(indexPath)
	if err != nil {
		return fmt.Errorf("loading json index file %s: %s", indexPath, err)
	}

	index.MergeIntoPackages(pm.packages)
	return nil
}

// Package looks for the Package with the given name, returning a structure
// able to perform further operations on that given resource
func (pm *PackageManager) Package(name string) *packageActions {
	//TODO: perhaps these 2 structure should be merged? cores.Packages vs pkgmgr??
	var err error
	thePackage := pm.packages.Packages[name]
	if thePackage == nil {
		err = fmt.Errorf("package '%s' not found", name)
	}
	return &packageActions{
		aPackage:     thePackage,
		forwardError: err,
	}
}

// Actions that can be done on a Package

// packageActions defines what actions can be performed on the specific Package
// It serves as a status container for the fluent APIs
type packageActions struct {
	aPackage     *cores.Package
	forwardError error
}

// Tool looks for the Tool with the given name, returning a structure
// able to perform further operations on that given resource
func (pa *packageActions) Tool(name string) *toolActions {
	var tool *cores.Tool
	err := pa.forwardError
	if err == nil {
		tool = pa.aPackage.Tools[name]

		if tool == nil {
			err = fmt.Errorf("tool '%s' not found in package '%s'", name, pa.aPackage.Name)
		}
	}
	return &toolActions{
		tool:         tool,
		forwardError: err,
	}
}

// END -- Actions that can be done on a Package

// Actions that can be done on a Tool

// toolActions defines what actions can be performed on the specific Tool
// It serves as a status container for the fluent APIs
type toolActions struct {
	tool         *cores.Tool
	forwardError error
}

// Get returns the final representation of the Tool
func (ta *toolActions) Get() (*cores.Tool, error) {
	err := ta.forwardError
	if err == nil {
		return ta.tool, nil
	}
	return nil, err
}

// IsInstalled checks whether any release of the Tool is installed in the system
func (ta *toolActions) IsInstalled() (bool, error) {
	if ta.forwardError != nil {
		return false, ta.forwardError
	}

	for _, release := range ta.tool.Releases {
		if release.IsInstalled() {
			return true, nil
		}
	}
	return false, nil
}

func (ta *toolActions) Release(version string) *toolReleaseActions {
	if ta.forwardError != nil {
		return &toolReleaseActions{forwardError: ta.forwardError}
	}
	release := ta.tool.GetRelease(version)
	if release == nil {
		return &toolReleaseActions{forwardError: fmt.Errorf("release %s not found for tool %s", version, ta.tool.String())}
	}
	return &toolReleaseActions{release: release}
}

// END -- Actions that can be done on a Tool

// toolReleaseActions defines what actions can be performed on the specific ToolRelease
// It serves as a status container for the fluent APIs
type toolReleaseActions struct {
	release      *cores.ToolRelease
	forwardError error
}

func (tr *toolReleaseActions) Get() (*cores.ToolRelease, error) {
	if tr.forwardError != nil {
		return nil, tr.forwardError
	}
	return tr.release, nil
}

func (pm *PackageManager) GetAllInstalledToolsReleases() []*cores.ToolRelease {
	tools := []*cores.ToolRelease{}
	for _, targetPackage := range pm.packages.Packages {
		for _, tool := range targetPackage.Tools {
			for _, release := range tool.Releases {
				if release.IsInstalled() {
					tools = append(tools, release)
				}
			}
		}
	}
	return tools
}

func (pm *PackageManager) FindToolsRequiredForBoard(board *cores.Board) ([]*cores.ToolRelease, error) {
	// core := board.Properties["build.core"]

	platform := board.PlatformRelease

	// maps "PACKAGER:TOOL" => ToolRelease
	foundTools := map[string]*cores.ToolRelease{}

	// a Platform may not specify required tools (because it's a platform that comes from a
	// sketchbook/hardware folder without a package_index.json) then add all available tools
	for _, targetPackage := range pm.packages.Packages {
		for _, tool := range targetPackage.Tools {
			rel := tool.GetLatestInstalled()
			if rel != nil {
				foundTools[rel.Tool.String()] = rel
			}
		}
	}

	// replace the default tools above with the specific required by the current platform
	for _, toolDep := range platform.Dependencies {
		tool := pm.FindToolDependency(toolDep)
		if tool == nil {
			return nil, fmt.Errorf("tool release not found: %s", toolDep)
		}
		foundTools[tool.Tool.String()] = tool
	}

	requiredTools := []*cores.ToolRelease{}
	for _, toolRel := range foundTools {
		requiredTools = append(requiredTools, toolRel)
	}
	return requiredTools, nil
}

func (pm *PackageManager) FindToolDependency(dep *cores.ToolDependency) *cores.ToolRelease {
	toolRelease, err := pm.Package(dep.ToolPackager).Tool(dep.ToolName).Release(dep.ToolVersion).Get()
	if err != nil {
		return nil
	}
	return toolRelease
}