/*
 * Copyright 2021-2025 Hewlett Packard Enterprise Development LP
 * Other additional copyright holders may be indicated within.
 *
 * The entirety of this work is licensed under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 *
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package helpers

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/NearNodeFlash/nnf-sos/pkg/command"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dwsv1alpha2 "github.com/DataWorkflowServices/dws/api/v1alpha2"
	nnfv1alpha5 "github.com/NearNodeFlash/nnf-sos/api/v1alpha5"
)

// Regex to scrape the progress output of the `dcp` command. Example output:
// Copied 1.000 GiB (10%) in 1.001 secs (4.174 GiB/s) 9 secs left ..."
var progressRe = regexp.MustCompile(`Copied.+\(([[:digit:]]{1,3})%\)`)

// Regexes to scrape the stats output of the `dcp` command once it's done. Example output:
/*
Copied 18.626 GiB (100%) in 171.434 secs (111.259 MiB/s) done
Copy data: 18.626 GiB (20000000000 bytes)
Copy rate: 111.258 MiB/s (20000000000 bytes in 171.434 seconds)
Syncing data to disk.
Sync completed in 0.017 seconds.
Fixing permissions.
Updated 2 items in 0.003 seconds (742.669 items/sec)
Syncing directory updates to disk.
Sync completed in 0.001 seconds.
Started: Jul-25-2024,16:44:33
Completed: Jul-25-2024,16:47:25
Seconds: 171.458
Items: 2
  Directories: 1
  Files: 1
  Links: 0
Data: 18.626 GiB (20000000000 bytes)
Rate: 111.243 MiB/s (20000000000 bytes in 171.458 seconds)
*/
type statsRegex struct {
	name  string
	regex *regexp.Regexp
}

var dcpStatsRegexes = []statsRegex{
	{"seconds", regexp.MustCompile(`Seconds: ([[:digit:].]+)`)},
	{"items", regexp.MustCompile(`Items: ([[:digit:]]+)`)},
	{"dirs", regexp.MustCompile(`Directories: ([[:digit:]]+)`)},
	{"files", regexp.MustCompile(`Files: ([[:digit:]]+)`)},
	{"links", regexp.MustCompile(`Files: ([[:digit:]]+)`)},
	{"data", regexp.MustCompile(`Data: (.*)`)},
	{"rate", regexp.MustCompile(`Rate: (.*)`)},
}

// Keep track of the context and its cancel function so that we can track
// and cancel data movement operations in progress
type DataMovementCancelContext struct {
	Ctx    context.Context
	Cancel context.CancelFunc
}

// Invalid error is a non-recoverable error type that implies the Data Movement resource is invalid
type invalidError struct {
	err error
}

func newInvalidError(format string, a ...any) *invalidError {
	return &invalidError{
		err: fmt.Errorf(format, a...),
	}
}

func (i *invalidError) Error() string { return i.err.Error() }
func (i *invalidError) Unwrap() error { return i.err }

func ParseDcpProgress(line string, cmdStatus *nnfv1alpha5.NnfDataMovementCommandStatus) error {
	match := progressRe.FindStringSubmatch(line)
	if len(match) > 0 {
		progress, err := strconv.Atoi(match[1])
		if err != nil {
			return fmt.Errorf("failed to parse progress output: %w", err)
		}
		progressInt32 := int32(progress)
		cmdStatus.ProgressPercentage = &progressInt32
	}

	return nil
}

// Walk through the output of the dcp command and remove all of the progress lines except for the
// first and last occurrences. Insert a snippet <snipped dcp progress output> to indicate that the
// output has been trimmed.
func TrimDcpProgressFromOutput(output string) string {
	threshold := 2
	progressFound := false
	firstProgressLine, lastProgressLine := -1, -1
	var result []string

	lines := strings.Split(output, "\n")

	// find all of the progress lines and keep track of the first and last
	for i, line := range lines {
		if progressRe.MatchString(line) {
			if !progressFound {
				firstProgressLine = i
				progressFound = true
			}
			lastProgressLine = i
		}
	}

	// nothing to do if there are 0-threshold progress lines
	if !progressFound || firstProgressLine == -1 || lastProgressLine == -1 || lastProgressLine-firstProgressLine <= threshold-1 {
		return output
	}

	insertedSnip := false
	for i, line := range lines {
		// keep the first and last progress lines
		if i == firstProgressLine || i == lastProgressLine {
			result = append(result, line)
			// remove all other progress lines
		} else if i > firstProgressLine && i < lastProgressLine {
			if !insertedSnip { // only insert the snip once
				result = append(result, "...")
				result = append(result, "<snipped dcp progress output>")
				result = append(result, "...")
				insertedSnip = true
			}
			// keep all other lines
		} else {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}

// Go through the list of dcp stat regexes, parse them, and put them in their appropriate place in cmdStatus
func ParseDcpStats(line string, cmdStatus *nnfv1alpha5.NnfDataMovementCommandStatus) error {
	for _, s := range dcpStatsRegexes {
		match := s.regex.FindStringSubmatch(line)
		if len(match) > 0 {
			matched := true // default case will set this to false

			// Each regex is parsed depending on its type (e.g. float, int, string) and then stored
			// in a different place in cmdStatus
			switch s.name {
			case "seconds":
				cmdStatus.Seconds = match[1]
			case "items":
				items, err := strconv.Atoi(match[1])
				if err != nil {
					return fmt.Errorf("failed to parse Items output: %w", err)
				}
				i32 := int32(items)
				cmdStatus.Items = &i32
			case "dirs":
				dirs, err := strconv.Atoi(match[1])
				if err != nil {
					return fmt.Errorf("failed to parse Directories output: %w", err)
				}
				i32 := int32(dirs)
				cmdStatus.Directories = &i32
			case "files":
				files, err := strconv.Atoi(match[1])
				if err != nil {
					return fmt.Errorf("failed to parse Directories output: %w", err)
				}
				i32 := int32(files)
				cmdStatus.Files = &i32
			case "links":
				links, err := strconv.Atoi(match[1])
				if err != nil {
					return fmt.Errorf("failed to parse Links output: %w", err)
				}
				i32 := int32(links)
				cmdStatus.Links = &i32
			case "data":
				cmdStatus.Data = match[1]
			case "rate":
				cmdStatus.Rate = match[1]
			default:
				matched = false // if we got here then nothing happened, so try the next regex
			}

			// if one of the regexes matched, then we're done
			if matched {
				return nil
			}
		}
	}

	return nil
}

func GetDMProfile(clnt client.Client, ctx context.Context, dm *nnfv1alpha5.NnfDataMovement) (*nnfv1alpha5.NnfDataMovementProfile, error) {

	var profile *nnfv1alpha5.NnfDataMovementProfile

	if dm.Spec.ProfileReference.Kind != reflect.TypeOf(nnfv1alpha5.NnfDataMovementProfile{}).Name() {
		return profile, fmt.Errorf("invalid NnfDataMovementProfile kind %s", dm.Spec.ProfileReference.Kind)
	}

	profile = &nnfv1alpha5.NnfDataMovementProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dm.Spec.ProfileReference.Name,
			Namespace: dm.Spec.ProfileReference.Namespace,
		},
	}
	if err := clnt.Get(ctx, client.ObjectKeyFromObject(profile), profile); err != nil {
		return nil, err
	}

	return profile, nil
}

func BuildDMCommand(profile *nnfv1alpha5.NnfDataMovementProfile, hostfile string, dm *nnfv1alpha5.NnfDataMovement, log logr.Logger) ([]string, error) {
	userConfig := dm.Spec.UserConfig != nil

	// If Dryrun is enabled, just use the "true" command
	if userConfig && dm.Spec.UserConfig.Dryrun {
		log.Info("Dry run detected")
		return []string{"true"}, nil
	}

	cmd := profile.Data.Command
	cmd = strings.ReplaceAll(cmd, "$HOSTFILE", hostfile)
	cmd = strings.ReplaceAll(cmd, "$UID", fmt.Sprintf("%d", dm.Spec.UserId))
	cmd = strings.ReplaceAll(cmd, "$GID", fmt.Sprintf("%d", dm.Spec.GroupId))
	cmd = strings.ReplaceAll(cmd, "$SRC", dm.Spec.Source.Path)
	cmd = strings.ReplaceAll(cmd, "$DEST", dm.Spec.Destination.Path)

	// Allow the user to override settings
	if userConfig {
		// Add extra mpirun options from the user
		if len(dm.Spec.UserConfig.MpirunOptions) > 0 {
			opts := dm.Spec.UserConfig.MpirunOptions

			// Insert the extra mpirun options after `mpirun`
			idx := strings.Index(cmd, "mpirun")
			if idx != -1 {
				idx += len("mpirun")
				cmd = cmd[:idx] + " " + opts + cmd[idx:]
			} else {
				log.Info("spec.config.mpirunOptions is set but `mpirun` is not in the DM command",
					"command", profile.Data.Command, "MpirunOptions", opts)
			}
		}

		// Add extra DCP options from the user
		if len(dm.Spec.UserConfig.DcpOptions) > 0 {
			opts := dm.Spec.UserConfig.DcpOptions

			// Insert the extra dcp options after `dcp`
			idx := strings.Index(cmd, "dcp")
			if idx != -1 {
				idx += len("dcp")
				cmd = cmd[:idx] + " " + opts + cmd[idx:]
			} else {
				log.Info("spec.config.dpcOptions is set but `dcp` is not found in the DM command",
					"command", profile.Data.Command, "DcpOptions", opts)
			}
		}
	}

	return strings.Split(cmd, " "), nil
}

func buildStatOrMkdirCommand(uid, gid uint32, cmd, hostfile, path string) string {
	cmd = strings.ReplaceAll(cmd, "$HOSTFILE", hostfile)
	cmd = strings.ReplaceAll(cmd, "$UID", fmt.Sprintf("%d", uid))
	cmd = strings.ReplaceAll(cmd, "$GID", fmt.Sprintf("%d", gid))
	cmd = strings.ReplaceAll(cmd, "$PATH", path)

	return cmd
}

func PrepareDestination(clnt client.Client, ctx context.Context, profile *nnfv1alpha5.NnfDataMovementProfile, dm *nnfv1alpha5.NnfDataMovement, mpiHostfile string, log logr.Logger) error {
	// These functions interact with the filesystem, so they can't run in the test env.  Also, if
	// the profile disables destination creation, skip it
	if isTestEnv() || !profile.Data.CreateDestDir {
		return nil
	}

	// Determine the destination directory based on the source and path
	log.Info("Determining destination directory based on source/dest file types")
	destDir, err := GetDestinationDir(profile, dm, mpiHostfile, log)
	if err != nil {
		return dwsv1alpha2.NewResourceError("could not determine source type").WithError(err).WithFatal()
	}

	// See if an index mount directory on the destination is required
	log.Info("Determining if index mount directory is required")
	indexMount, err := checkIndexMountDir(clnt, ctx, dm)
	if err != nil {
		return dwsv1alpha2.NewResourceError("could not determine index mount directory").WithError(err).WithFatal()
	}

	// Account for index mount directory on the destDir and the dm dest path
	// This updates the destination on dm
	if indexMount != "" {
		log.Info("Index mount directory is required", "indexMountdir", indexMount)
		d, err := HandleIndexMountDir(profile, dm, destDir, indexMount, mpiHostfile, log)
		if err != nil {
			return dwsv1alpha2.NewResourceError("could not handle index mount directory").WithError(err).WithFatal()
		}
		destDir = d
		log.Info("Updated destination for index mount directory", "destDir", destDir, "dm.Spec.Destination.Path", dm.Spec.Destination.Path)
	}

	// Create the destination directory
	log.Info("Creating destination directory", "destinationDir", destDir, "indexMountDir", indexMount)
	if err := createDestinationDir(profile, dm, destDir, mpiHostfile, log); err != nil {
		return dwsv1alpha2.NewResourceError("could not create destination directory").WithError(err).WithFatal()
	}

	log.Info("Destination prepared", "dm.Spec.Destination", dm.Spec.Destination)
	return nil

}

// Check for a copy_out situation by looking at the source filesystem's type. If it's gfs2 or xfs,
// then we need to account for a Fan-In situation and create index mount directories on the
// destination. Returns the index mount directory from the source path.
func checkIndexMountDir(clnt client.Client, ctx context.Context, dm *nnfv1alpha5.NnfDataMovement) (string, error) {
	var storage *nnfv1alpha5.NnfStorage
	var nodeStorage *nnfv1alpha5.NnfNodeStorage

	if dm.Spec.Source.StorageReference.Kind == reflect.TypeOf(nnfv1alpha5.NnfStorage{}).Name() {
		// The source storage reference is NnfStorage - this came from copy_in/copy_out directives
		storageRef := dm.Spec.Source.StorageReference

		storage = &nnfv1alpha5.NnfStorage{}
		if err := clnt.Get(ctx, types.NamespacedName{Name: storageRef.Name, Namespace: storageRef.Namespace}, storage); err != nil {
			if apierrors.IsNotFound(err) {
				return "", newInvalidError("could not retrieve NnfStorage for checking index mounts: %s", err.Error())
			}
			return "", err
		}
	} else if dm.Spec.Source.StorageReference.Kind == reflect.TypeOf(nnfv1alpha5.NnfNodeStorage{}).Name() {
		// The source storage reference is NnfNodeStorage - this came from copy_offload
		storageRef := dm.Spec.Source.StorageReference

		nodeStorage = &nnfv1alpha5.NnfNodeStorage{}
		if err := clnt.Get(ctx, types.NamespacedName{Name: storageRef.Name, Namespace: storageRef.Namespace}, nodeStorage); err != nil {
			if apierrors.IsNotFound(err) {
				return "", newInvalidError("could not retrieve NnfNodeStorage for checking index mounts: %s", err.Error())
			}
			return "", err
		}
	} else {
		return "", nil // nothing to do here
	}

	// If it is gfs2 or xfs, then there are index mounts
	if storage != nil && (storage.Spec.FileSystemType == "gfs2" || storage.Spec.FileSystemType == "xfs") {
		return ExtractIndexMountDir(dm.Spec.Source.Path, dm.Namespace)
	} else if nodeStorage != nil && (nodeStorage.Spec.FileSystemType == "gfs2" || nodeStorage.Spec.FileSystemType == "xfs") {
		return ExtractIndexMountDir(dm.Spec.Source.Path, dm.Namespace)
	}

	return "", nil // nothing to do here
}

// Pull out the index mount directory from the path for the correct file systems that require it
func ExtractIndexMountDir(path, namespace string) (string, error) {

	// To get the index mount directory, We need to scrape the source path to find the index mount
	// directory - we don't have access to the directive index and only know the source/dest paths.
	// Match namespace-digit and then the end of the string OR slash
	pattern := regexp.MustCompile(fmt.Sprintf(`(%s-\d+)(/|$)`, namespace))
	match := pattern.FindStringSubmatch(path)
	if match == nil {
		return "", fmt.Errorf("could not extract index mount directory from source path: %s", path)
	}

	idxMount := match[1]
	// If the path ends with the index mount (and no trailing slash), then we need to return "" so
	// that we don't double up. This happens when you copy out of the root without a trailing slash
	// - dcp will copy the directory (index mount) over to the destination. So there's no need to
	// append it.
	if strings.HasSuffix(path, idxMount) {
		return "", nil // nothing to do here
	}

	return idxMount, nil
}

// Given a destination directory and index mount directory, apply the necessary changes to the
// destination directory and the DM's destination path to account for index mount directories
func HandleIndexMountDir(profile *nnfv1alpha5.NnfDataMovementProfile, dm *nnfv1alpha5.NnfDataMovement, destDir, indexMount, mpiHostfile string, log logr.Logger) (string, error) {

	// For cases where the root directory (e.g. $DW_JOB_my_workflow) is supplied, the path will end
	// in the index mount directory without a trailing slash. If that's the case, there's nothing
	// to handle.
	if strings.HasSuffix(dm.Spec.Source.Path, indexMount) {
		return destDir, nil
	}

	// Add index mount directory to the end of the destination directory
	idxMntDir := filepath.Join(destDir, indexMount)

	// Update dm.Spec.Destination.Path for the new path
	dmDest := idxMntDir

	// Get the source to see if it's a file
	srcIsFile, err := isSourceAFile(profile, dm, mpiHostfile, log)
	if err != nil {
		return "", err
	}
	destIsFile := isDestAFile(profile, dm, mpiHostfile, log)
	log.Info("Checking source and dest types", "srcIsFile", srcIsFile, "destIsFile", destIsFile)

	// Account for file-file
	if srcIsFile && destIsFile {
		dmDest = filepath.Join(idxMntDir, filepath.Base(dm.Spec.Destination.Path))
	}

	// Preserve the trailing slash. This should not matter in dcp's eye, but let's preserve it for
	// posterity.
	if strings.HasSuffix(dm.Spec.Destination.Path, "/") {
		dmDest += "/"
	}

	// Update the dm destination with the new path
	dm.Spec.Destination.Path = dmDest

	return idxMntDir, nil
}

// Determine the directory path to create based on the source and destination.
// Returns the mkdir directory and error.
func GetDestinationDir(profile *nnfv1alpha5.NnfDataMovementProfile, dm *nnfv1alpha5.NnfDataMovement, mpiHostfile string, log logr.Logger) (string, error) {
	// Default to using the full path of dest
	destDir := dm.Spec.Destination.Path

	srcIsFile, err := isSourceAFile(profile, dm, mpiHostfile, log)
	if err != nil {
		return "", err
	}

	// Account for file-file data movement - we don't want the full path
	// ex: /path/to/a/file -> /path/to/a
	if srcIsFile && isDestAFile(profile, dm, mpiHostfile, log) {
		destDir = filepath.Dir(dm.Spec.Destination.Path)
	}

	// We know it's a directory, so we don't care about the trailing slash
	return filepath.Clean(destDir), nil
}

// Use mpirun to run stat on a file as a given UID/GID by using `setpriv`
func mpiStat(profile *nnfv1alpha5.NnfDataMovementProfile, path string, uid, gid uint32, mpiHostfile string, log logr.Logger) (string, error) {
	cmd := buildStatOrMkdirCommand(uid, gid, profile.Data.StatCommand, mpiHostfile, path)

	output, err := command.Run(cmd, log)
	if err != nil {
		return output, fmt.Errorf("could not stat path ('%s'): %w", path, err)
	}
	log.Info("mpiStat", "command", cmd, "path", path, "output", output)

	return output, nil
}

// Use mpirun to determine if a given path is a directory
func mpiIsDir(profile *nnfv1alpha5.NnfDataMovementProfile, path string, uid, gid uint32, mpiHostfile string, log logr.Logger) (bool, error) {
	output, err := mpiStat(profile, path, uid, gid, mpiHostfile, log)
	if err != nil {
		return false, err
	}

	if strings.Contains(strings.ToLower(output), "directory") {
		log.Info("mpiIsDir", "path", path, "directory", true)
		return true, nil
	} else if strings.Contains(strings.ToLower(output), "file") {
		log.Info("mpiIsDir", "path", path, "directory", false)
		return false, nil
	} else if os.Getenv("ENVIRONMENT") == "kind" && output == "symbolic link\n" {
		// In KIND it will be a symlink to a directory.
		log.Info("mpiIsDir using symlink in KIND", "path", path)
		return true, nil
	} else {
		return false, fmt.Errorf("could not determine file type of path ('%s'): %s", path, output)
	}
}

// Check to see if the source path is a file. The source must exist and will result in error if it
// does not. Do not use mpi in the test environment.
func isSourceAFile(profile *nnfv1alpha5.NnfDataMovementProfile, dm *nnfv1alpha5.NnfDataMovement, mpiHostFile string, log logr.Logger) (bool, error) {
	var isDir bool
	var err error

	if isTestEnv() {
		sf, err := os.Stat(dm.Spec.Source.Path)
		if err != nil {
			return false, fmt.Errorf("source file does not exist: %w", err)
		}
		isDir = sf.IsDir()
	} else {
		isDir, err = mpiIsDir(profile, dm.Spec.Source.Path, dm.Spec.UserId, dm.Spec.GroupId, mpiHostFile, log)
		if err != nil {
			return false, err
		}
	}

	return !isDir, nil
}

// Check to see if the destination path is a file. If it exists, use stat to determine if it is a
// file or a directory. If it doesn't exist, check for a trailing slash and make an assumption based
// on that.
func isDestAFile(profile *nnfv1alpha5.NnfDataMovementProfile, dm *nnfv1alpha5.NnfDataMovement, mpiHostFile string, log logr.Logger) bool {
	isFile := false
	exists := true
	dest := dm.Spec.Destination.Path

	// Attempt to the get the destination file. The error is not important since an assumption can
	// be made by checking if there is a trailing slash.
	if isTestEnv() {
		df, _ := os.Stat(dest)
		if df != nil { // file exists, so use IsDir()
			isFile = !df.IsDir()
		} else {
			exists = false
		}
	} else {
		isDir, err := mpiIsDir(profile, dest, dm.Spec.UserId, dm.Spec.GroupId, mpiHostFile, log)
		if err == nil { // file exists, so use mpiIsDir() result
			isFile = !isDir
		} else {
			exists = false
		}
	}

	// Dest does not exist but looks like a file (no trailing slash)
	if !exists && !strings.HasSuffix(dest, "/") {
		log.Info("Destination does not exist and has no trailing slash - assuming file", "dest", dest)
		isFile = true
	}

	return isFile
}

func createDestinationDir(profile *nnfv1alpha5.NnfDataMovementProfile, dm *nnfv1alpha5.NnfDataMovement, dest, mpiHostfile string, log logr.Logger) error {

	// Use mpirun to check if a file exists
	mpiExists := func() (bool, error) {
		_, err := mpiStat(profile, dest, dm.Spec.UserId, dm.Spec.GroupId, mpiHostfile, log)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "no such file or directory") {
				return false, nil
			}
			return false, err
		}

		return true, nil
	}

	// Don't do anything if it already exists
	if exists, err := mpiExists(); err != nil {
		return err
	} else if exists {
		return nil
	}

	cmd := buildStatOrMkdirCommand(dm.Spec.UserId, dm.Spec.GroupId, profile.Data.MkdirCommand, mpiHostfile, dest)
	output, err := command.Run(cmd, log)
	if err != nil {
		return fmt.Errorf("data movement mkdir failed ('%s'): %w output: %s", cmd, err, output)
	}

	return nil
}

// Create an MPI hostfile given settings from a profile and user config from the dm
func CreateMpiHostfile(profile *nnfv1alpha5.NnfDataMovementProfile, hosts []string, dm *nnfv1alpha5.NnfDataMovement) (string, error) {
	userConfig := dm.Spec.UserConfig != nil

	// Create MPI hostfile only if included in the provided command
	slots := profile.Data.Slots
	maxSlots := profile.Data.MaxSlots

	// Allow the user to override the slots and max_slots in the hostfile.
	if userConfig && dm.Spec.UserConfig.Slots != nil && *dm.Spec.UserConfig.Slots >= 0 {
		slots = *dm.Spec.UserConfig.Slots
	}
	if userConfig && dm.Spec.UserConfig.MaxSlots != nil && *dm.Spec.UserConfig.MaxSlots >= 0 {
		maxSlots = *dm.Spec.UserConfig.MaxSlots
	}

	// Create it
	hostfile, err := WriteMpiHostfile(dm.Name, hosts, slots, maxSlots)
	if err != nil {
		return "", err
	}

	return hostfile, nil
}

// Create the MPI Hostfile given a list of hosts, slots, and maxSlots. A temporary directory is
// created based on the DM Name. The hostfile is created inside of this directory.
// A value of 0 for slots or maxSlots will not use it in the hostfile.
func WriteMpiHostfile(dmName string, hosts []string, slots, maxSlots int) (string, error) {

	tmpdir := filepath.Join("/tmp", dmName)
	if err := os.MkdirAll(tmpdir, 0755); err != nil {
		return "", err
	}
	hostfilePath := filepath.Join(tmpdir, "hostfile")

	f, err := os.Create(hostfilePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// $ cat my-hosts
	// node0 slots=2 max_slots=20
	// node1 slots=2 max_slots=20
	// https://www.open-mpi.org/faq/?category=running#mpirun-hostfile
	unique_hosts := map[string]int{}
	for _, host := range hosts {
		// hostfiles cannot list a host more than once - this can happen when there are multiple
		// OSTs on a single rabbit. Move on if a host is already present.
		if _, found := unique_hosts[host]; found {
			continue
		}
		unique_hosts[host] = 1

		if _, err := f.WriteString(host); err != nil {
			return "", err
		}

		if slots > 0 {
			_, err := f.WriteString(fmt.Sprintf(" slots=%d", slots))
			if err != nil {
				return "", err
			}
		}

		if maxSlots > 0 {
			_, err := f.WriteString(fmt.Sprintf(" max_slots=%d", maxSlots))
			if err != nil {
				return "", err
			}
		}

		_, err = f.WriteString("\n")
		if err != nil {
			return "", err
		}
	}

	return hostfilePath, nil
}

// Get the first line of the hostfile for verification
func PeekMpiHostfile(hostfile string) string {
	file, err := os.Open(hostfile)
	if err != nil {
		return ""
	}
	defer file.Close()

	str, err := bufio.NewReader(file).ReadString('\n')
	if err != nil {
		return ""
	}

	return str
}

func ProgressCollectionEnabled(collectInterval time.Duration) bool {
	return collectInterval >= 1*time.Second
}

func isTestEnv() bool {
	_, found := os.LookupEnv("NNF_TEST_ENVIRONMENT")
	return found
}

// Retrieve the NNF Nodes that are the target of the data movement operation
func GetStorageNodeNames(clnt client.Client, ctx context.Context, dm *nnfv1alpha5.NnfDataMovement) ([]string, error) {
	// If this is a node data movement request simply reference the localhost
	if dm.Namespace == os.Getenv("NNF_NODE_NAME") || isTestEnv() {
		return []string{"localhost"}, nil
	}

	// Otherwise, this is a system wide data movement request we target the NNF Nodes that are defined in the storage specification
	var storageRef corev1.ObjectReference
	if dm.Spec.Source.StorageReference.Kind == reflect.TypeOf(nnfv1alpha5.NnfStorage{}).Name() {
		storageRef = dm.Spec.Source.StorageReference
	} else if dm.Spec.Destination.StorageReference.Kind == reflect.TypeOf(nnfv1alpha5.NnfStorage{}).Name() {
		storageRef = dm.Spec.Destination.StorageReference
	} else {
		return nil, newInvalidError("Neither source or destination is of NNF Storage type")
	}

	storage := &nnfv1alpha5.NnfStorage{}
	if err := clnt.Get(ctx, types.NamespacedName{Name: storageRef.Name, Namespace: storageRef.Namespace}, storage); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, newInvalidError("NNF Storage not found: %s", err.Error())
		}
		return nil, err
	}

	if storage.Spec.FileSystemType != "lustre" {
		return nil, newInvalidError("Unsupported storage type %s", storage.Spec.FileSystemType)
	}
	targetAllocationSetIndex := -1
	for allocationSetIndex, allocationSet := range storage.Spec.AllocationSets {
		if allocationSet.TargetType == "ost" {
			targetAllocationSetIndex = allocationSetIndex
		}
	}

	if targetAllocationSetIndex == -1 {
		return nil, newInvalidError("ost allocation set not found")
	}

	nodes := storage.Spec.AllocationSets[targetAllocationSetIndex].Nodes
	nodeNames := make([]string, len(nodes))
	for idx := range nodes {
		nodeNames[idx] = nodes[idx].Name
	}

	return nodeNames, nil
}

func GetWorkerHostnames(clnt client.Client, ctx context.Context, nodes []string) ([]string, error) {

	if nodes[0] == "localhost" {
		return nodes, nil
	}

	// For this first iteration, we need to look up the Pods associated with the MPI workers on each
	// individual rabbit, mapping the nodename to a worker IP address. Since we've set up a headless
	// service matching the subdomain, the worker's IP is used as the DNS name (substituting '-' for '.')
	// following the description here:
	// https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/#pod-s-hostname-and-subdomain-fields
	//
	// Ideally this look-up would not be required if the MPI worker pods could have the same hostname
	// as the nodename. There is no straightfoward way for this to happen although it has been raised
	// several times in the k8s community.
	//
	// A couple of ideas on how to support this...
	// 1. Using an initContainer which would get the parent pod and modify the hostname.
	// 2. Not use a DaemonSet to create the MPI worker pods, but do so manually, assigning
	//    the correct hostname to each pod. Right now the daemon set provides scheduling and
	//    pod restarts, and we would lose this feature if we managed the pods individually.

	// Get the Rabbit DM Worker Pods
	listOptions := []client.ListOption{
		client.InNamespace(nnfv1alpha5.DataMovementNamespace),
		client.MatchingLabels(map[string]string{
			nnfv1alpha5.DataMovementWorkerLabel: "true",
		}),
	}

	pods := &corev1.PodList{}
	if err := clnt.List(ctx, pods, listOptions...); err != nil {
		return nil, err
	}

	nodeNameToHostnameMap := map[string]string{}
	for _, pod := range pods.Items {
		nodeNameToHostnameMap[pod.Spec.NodeName] = strings.ReplaceAll(pod.Status.PodIP, ".", "-") + ".dm." + nnfv1alpha5.DataMovementNamespace // TODO: make the subdomain const TODO: use nnf-dm-system const
	}

	hostnames := make([]string, len(nodes))
	for idx := range nodes {

		hostname, found := nodeNameToHostnameMap[nodes[idx]]
		if !found {
			return nil, newInvalidError("Hostname invalid for node %s", nodes[idx])
		}

		hostnames[idx] = hostname
	}

	return hostnames, nil
}
