package bacalhau

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/filecoin-project/bacalhau/pkg/executor"
	"github.com/filecoin-project/bacalhau/pkg/job"
	"github.com/filecoin-project/bacalhau/pkg/storage"
	"github.com/filecoin-project/bacalhau/pkg/system"
	"github.com/filecoin-project/bacalhau/pkg/verifier"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var jobspec *executor.JobSpec
var filename string
var jobConcurrency int
var jobInputUrls []string
var jobInputVolumes []string
var jobOutputVolumes []string
var jobWorkingDir string
var jobLabels []string

func init() { //nolint:gochecknoinits
	applyCmd.PersistentFlags().StringVarP(
		&filename, "filename", "f", "",
		`Path to the job file`,
	)

	applyCmd.PersistentFlags().IntVarP(
		&jobConcurrency, "concurrency", "c", 1,
		`How many nodes should run the job in parallel`,
	)

	applyCmd.PersistentFlags().StringSliceVarP(&jobLabels,
		"labels", "l", []string{},
		`List of jobTags for the job. In the format 'a,b,c,1'. All characters not matching /a-zA-Z0-9_:|-/ and all emojis will be stripped.`,
	)
}

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Submit a job.json or job.yaml file and run it on the network",
	Args:  cobra.MinimumNArgs(0),
	RunE: func(cmd *cobra.Command, cmdArgs []string) error { //nolint:unparam // incorrect that cmd is unused.
		ctx := context.Background()
		fileextension := filepath.Ext(filename)

		fileContent, err := os.Open(filename)

		if err != nil {
			return err
		}

		defer fileContent.Close()

		byteResult, err := io.ReadAll(fileContent)

		if err != nil {
			return err
		}

		if fileextension == ".json" {
			err = json.Unmarshal(byteResult, &jobspec)
			if err != nil {
				return err
			}
		}

		if fileextension == ".yaml" || fileextension == ".yml" {
			err = yaml.Unmarshal(byteResult, &jobspec)
			if err != nil {
				return err
			}
		}

		jobImage := jobspec.Docker.Image

		jobEntrypoint := jobspec.Docker.Entrypoint

		if len(jobspec.Inputs) != 0 {
			for _, jobspecInput := range jobspec.Inputs {
				var storageSpecEngineType storage.StorageSourceType
				storageSpecEngineType, err = storage.ParseStorageSourceType(jobspecInput.EngineName)
				if err != nil {
					return err
				}
				if jobspecInput.Path == "" {
					return fmt.Errorf("empty volume mount point %+v", jobspecInput)
				}
				if storageSpecEngineType == storage.StorageSourceIPFS {
					if jobspecInput.Cid == "" {
						return fmt.Errorf("empty ipfs volume cid %+v", jobspecInput)
					}
					is := jobspecInput.Cid + ":" + jobspecInput.Path
					jobInputVolumes = append(jobInputVolumes, is)
				} else if storageSpecEngineType == storage.StorageSourceURLDownload {
					if jobspecInput.URL == "" {
						return fmt.Errorf("empty url volume url %+v", jobspecInput)
					}
					is := jobspecInput.URL + ":" + jobspecInput.Path
					jobInputUrls = append(jobInputUrls, is)
				} else {
					return fmt.Errorf("unknown storage source type %s", jobspecInput.EngineName)
				}
			}
		}

		if len(jobspec.Outputs) != 0 {
			for _, jobspecsOutputs := range jobspec.Outputs {
				is := jobspecsOutputs.Name + ":" + jobspecsOutputs.Path
				jobOutputVolumes = append(jobOutputVolumes, is)
			}
		}
		jobOutputVolumes = append(jobOutputVolumes, "outputs:/outputs")

		engineType, err := executor.ParseEngineType(jobspec.EngineName)
		if err != nil {
			cmd.Printf("Error parsing engine type: %s", err)
			return err
		}

		verifierType, err := verifier.ParseVerifierType(jobspec.VerifierName)
		if err != nil {
			cmd.Printf("Error parsing verifier type: %s", err)
			return err
		}

		if len(jobWorkingDir) > 0 {
			err = system.ValidateWorkingDir(jobWorkingDir)
			if err != nil {
				return err
			}
		}

		spec, deal, err := job.ConstructDockerJob(
			engineType,
			verifierType,
			jobspec.Resources.CPU,
			jobspec.Resources.Memory,
			jobspec.Resources.GPU,
			jobInputUrls,
			jobInputVolumes,
			jobOutputVolumes,
			jobspec.Docker.Env,
			jobEntrypoint,
			jobImage,
			jobConcurrency,
			jobLabels,
			jobWorkingDir,
			doNotTrack,
		)
		if err != nil {
			return err
		}

		job, err := getAPIClient().Submit(ctx, spec, deal, nil)
		if err != nil {
			return err
		}

		cmd.Printf("%s\n", job.ID)
		return nil

	},
}
