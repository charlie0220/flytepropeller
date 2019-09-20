package dynamic

import (
	"context"
	"testing"

	mocks2 "github.com/lyft/flytepropeller/pkg/controller/nodes/handler/mocks"

	"github.com/lyft/flyteidl/gen/pb-go/flyteidl/core"

	"github.com/stretchr/testify/mock"

	"github.com/stretchr/testify/assert"
)

func TestHierarchicalNodeID(t *testing.T) {
	t.Run("empty parent", func(t *testing.T) {
		actual, err := hierarchicalNodeID("", "abc")
		assert.NoError(t, err)
		assert.Equal(t, "-abc", actual)
	})

	t.Run("long result", func(t *testing.T) {
		actual, err := hierarchicalNodeID("abcdefghijklmnopqrstuvwxyz", "abc")
		assert.NoError(t, err)
		assert.Equal(t, "fpa3kc3y", actual)
	})
}

func TestUnderlyingInterface(t *testing.T) {
	expectedIface := &core.TypedInterface{
		Outputs: &core.VariableMap{
			Variables: map[string]*core.Variable{
				"in": {
					Type: &core.LiteralType{
						Type: &core.LiteralType_Simple{
							Simple: core.SimpleType_INTEGER,
						},
					},
				},
			},
		},
	}

	tk := &core.TaskTemplate{
		Interface: expectedIface,
	}

	tr := &mocks2.TaskReader{}
	tr.On("Read", mock.Anything).Return(tk, nil)

	iface, err := underlyingInterface(context.TODO(), tr)
	assert.NoError(t, err)
	assert.NotNil(t, iface)
	assert.Equal(t, expectedIface, iface)

	tk.Interface = nil
	iface, err = underlyingInterface(context.TODO(), tr)
	assert.NoError(t, err)
	assert.NotNil(t, iface)
	assert.Nil(t, iface.Outputs)
}

func createInmemoryStore(t testing.TB) *storage.DataStore {
	cfg := storage.Config{
		Type: storage.TypeMemory,
	}

	d, err := storage.NewDataStore(&cfg, promutils.NewTestScope())
	assert.NoError(t, err)

	return d
}

func Test_cacheFlyteWorkflow(t *testing.T) {
	store := createInmemoryStore(t)
	expected := &v1alpha1.FlyteWorkflow{
		TypeMeta:   v1.TypeMeta{},
		ObjectMeta: v1.ObjectMeta{},
		WorkflowSpec: &v1alpha1.WorkflowSpec{
			ID: "abc",
			Connections: v1alpha1.Connections{
				DownstreamEdges: map[v1alpha1.NodeID][]v1alpha1.NodeID{},
				UpstreamEdges:   map[v1alpha1.NodeID][]v1alpha1.NodeID{},
			},
		},
	}

	location := storage.DataReference("somekey/file.json")
	assert.NoError(t, cacheFlyteWorkflow(context.TODO(), store, expected, location))
	actual, err := loadCachedFlyteWorkflow(context.TODO(), store, location)
	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
}
