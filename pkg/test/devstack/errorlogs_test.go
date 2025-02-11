package devstack

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/filecoin-project/bacalhau/pkg/computenode"
	"github.com/filecoin-project/bacalhau/pkg/executor"
	"github.com/filecoin-project/bacalhau/pkg/job"
	_ "github.com/filecoin-project/bacalhau/pkg/logger"
	"github.com/filecoin-project/bacalhau/pkg/publicapi"
	"github.com/filecoin-project/bacalhau/pkg/system"
	"github.com/filecoin-project/bacalhau/pkg/verifier"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type DevstackErrorLogsSuite struct {
	suite.Suite
}

// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
func TestDevstackErrorLogsSuite(t *testing.T) {
	suite.Run(t, new(DevstackErrorLogsSuite))
}

// Before all suite
func (suite *DevstackErrorLogsSuite) SetupAllSuite() {

}

// Before each test
func (suite *DevstackErrorLogsSuite) SetupTest() {
	system.InitConfigForTesting(suite.T())
}

func (suite *DevstackErrorLogsSuite) TearDownTest() {
}

func (suite *DevstackErrorLogsSuite) TearDownAllSuite() {

}
func (suite *DevstackErrorLogsSuite) TestErrorContainer() {
	suite.T().Skip("REMOVE_WHEN_OUTPUTDIRECTORY_QUESTION_ANSWERED https://github.com/filecoin-project/bacalhau/issues/388")

	stdout := "apples"
	stderr := "oranges"
	exitCode := "19"

	ctx, span := newSpan("TestErrorContainer")
	defer span.End()

	stack, cm := SetupTest(
		suite.T(),
		1,
		0,
		computenode.NewDefaultComputeNodeConfig(),
	)
	defer TeardownTest(stack, cm)

	nodeIDs, err := stack.GetNodeIds()
	require.NoError(suite.T(), err)

	jobSpec := executor.JobSpec{
		Engine:   executor.EngineDocker,
		Verifier: verifier.VerifierIpfs,
		Docker: executor.JobSpecDocker{
			Image: "ubuntu",
			Entrypoint: []string{
				"bash",
				"-c",
				fmt.Sprintf("echo %s && >&2 echo %s && exit %s", stdout, stderr, exitCode),
			},
		},
	}

	jobDeal := executor.JobDeal{
		Concurrency: 1,
	}

	apiUri := stack.Nodes[0].APIServer.GetURI()
	apiClient := publicapi.NewAPIClient(apiUri)
	submittedJob, err := apiClient.Submit(ctx, jobSpec, jobDeal, nil)
	require.NoError(suite.T(), err)

	resolver := apiClient.GetJobStateResolver()

	err = resolver.Wait(
		ctx,
		submittedJob.ID,
		len(nodeIDs),
		job.WaitThrowErrors([]executor.JobStateType{
			executor.JobStateCancelled,
			executor.JobStateError,
		}),
		job.WaitForJobStates(map[executor.JobStateType]int{
			executor.JobStateError: len(nodeIDs),
		}),
	)
	require.NoError(suite.T(), err)

	shards, err := resolver.GetShards(ctx, submittedJob.ID)
	require.NoError(suite.T(), err)
	require.True(suite.T(), len(shards) > 0)

	state := shards[0]

	outputDir, err := ioutil.TempDir("", "bacalhau-ipfs-devstack-test")
	require.NoError(suite.T(), err)

	node, err := stack.GetNode(ctx, nodeIDs[0])
	require.NoError(suite.T(), err)

	outputPath := filepath.Join(outputDir, state.ResultsID)
	err = node.IpfsClient.Get(ctx, state.ResultsID, outputPath)
	require.NoError(suite.T(), err)

	stdoutBytes, err := os.ReadFile(fmt.Sprintf("%s/stdout", outputPath))
	require.NoError(suite.T(), err)
	stderrBytes, err := os.ReadFile(fmt.Sprintf("%s/stderr", outputPath))
	require.NoError(suite.T(), err)
	exitCodeBytes, err := os.ReadFile(fmt.Sprintf("%s/exitCode", outputPath))
	require.NoError(suite.T(), err)

	require.Equal(suite.T(), stdout, strings.TrimSpace(string(stdoutBytes)))
	require.Equal(suite.T(), stderr, strings.TrimSpace(string(stderrBytes)))
	require.Equal(suite.T(), exitCode, strings.TrimSpace(string(exitCodeBytes)))
}
