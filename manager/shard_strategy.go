package manager

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/flyteorg/flytepropeller/manager/config"

	v1 "k8s.io/api/core/v1"
)

const ConsistentHashingKeyspaceSize = 32;

type ShardStrategy interface {
	GetPodCount() (int, error)
	UpdatePodSpec(pod *v1.PodSpec, podIndex int) error
}

func NewShardStrategy(ctx context.Context, shardConfig config.ShardConfig) (ShardStrategy, error) {
	switch shardConfig.Type {
	case config.ConsistentHashingShardType:
		if shardConfig.PodCount <= 0 {
			return nil, fmt.Errorf("configured PodCount (%d) must be greater than zero", shardConfig.PodCount)
		} else if shardConfig.PodCount > ConsistentHashingKeyspaceSize {
			return nil, fmt.Errorf("configured PodCount (%d) is larger than available keyspace size (%d)", shardConfig.PodCount, ConsistentHashingKeyspaceSize)
		}

		return &ConsistentHashingShardStrategy{
			podCount: shardConfig.PodCount,
		}, nil
	case config.NamespaceShardType:
		namespaceReplicas := make([][]string, 0)
		for _, namespaceReplica := range shardConfig.NamespaceReplicas {
			if len(namespaceReplica.Namespaces) == 0 {
				return nil, fmt.Errorf("unable to create namespace replica with 0 configured namespace(s)")
			}

			namespaceReplicas = append(namespaceReplicas, namespaceReplica.Namespaces)
		}

		return &NamespaceShardStrategy{
			namespaceReplicas: namespaceReplicas,
		}, nil
	}

	return nil, fmt.Errorf("shard strategy '%s' does not exist", shardConfig.Type)
}

// The ConsistentHashingShardStrategy load-balances FlyteWorkflow processing by evenly distributing
// keyspace range responsibilities between a collection of pods. Each keyspace token is assigned to
// a single FlytePropeller controller ensuring deterministic processing.
type ConsistentHashingShardStrategy struct {
	podCount int
}

func (c *ConsistentHashingShardStrategy) GetPodCount() (int, error) {
	return c.podCount, nil
}

func (c *ConsistentHashingShardStrategy) UpdatePodSpec(pod *v1.PodSpec, podIndex int) error {
	container, err := getFlytePropellerContainer(pod)
	if err != nil {
		return err
	}

	startKey, endKey := computeKeyRange(ConsistentHashingKeyspaceSize, c.podCount, podIndex)
	for i := startKey; i < endKey; i++ {
		container.Args = append(container.Args, "--propeller.include-shard-label", fmt.Sprintf("%d", i))
	}

	return nil
}

// Computes a [startKey, endKey) pair denoting the key responsibilities for the provided pod index
// given the keyspaceSize and podCount parameters
func computeKeyRange(keyspaceSize, podCount, podIndex int) (int, int) {
	keysPerPod := int(math.Floor(float64(keyspaceSize / podCount)))
	keyRemainder := keyspaceSize % keysPerPod

	return computeStartKey(keysPerPod, keyRemainder, podIndex), computeStartKey(keysPerPod, keyRemainder, podIndex+1)
}

func computeStartKey(keysPerPod, keysRemainder, podIndex int) int {
	return (intMin(podIndex, keysRemainder) * (keysPerPod + 1)) + (intMax(0, podIndex - keysRemainder) * keysPerPod)
}

func intMin(a, b int) int {
	if a < b {
		return a
	}

	return b
}

func intMax(a, b int) int {
	if a > b {
		return a
	}

	return b
}

// The NamespaceShardStrategy distributes FlyteWorkflow processing using namespace labels. Each
// FlytePropeller instance is responsible for FlyteWorkflows defined within the configured
// namespace(s).
type NamespaceShardStrategy struct {
	namespaceReplicas [][]string
}

func (n *NamespaceShardStrategy) GetPodCount() (int, error) {
	return len(n.namespaceReplicas), nil
}

func (n *NamespaceShardStrategy) UpdatePodSpec(pod *v1.PodSpec, podIndex int) error {
	container, err := getFlytePropellerContainer(pod)
	if err != nil {
		return err
	}

	for _, namespace := range n.namespaceReplicas[podIndex] {
		container.Args = append(container.Args, "--propeller.include-namespace-label", fmt.Sprintf("%s", namespace))
	}

	return nil
}

func getFlytePropellerContainer(pod *v1.PodSpec) (*v1.Container, error) {
	// find flytepropeller container(s)
	var containers []*v1.Container
	for i := 0; i < len(pod.Containers); i++ {
		commands := pod.Containers[i].Command
		if len(commands) > 0 && commands[0] == "flytepropeller" {
			containers = append(containers, &pod.Containers[i])
		}
	}

	if len(containers) != 1 {
		return nil, errors.New(fmt.Sprintf("expecting 1 flytepropeller container in podtemplate but found %d, ", len(containers)))
	}

	return containers[0], nil
}
