package webhook

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/flyteorg/flyteidl/gen/pb-go/flyteidl/core"
	"github.com/flyteorg/flytestdlib/logger"
	corev1 "k8s.io/api/core/v1"
)

const (
	AWSSecretArnAnnotation       = "secrets.k8s.aws/secret-arn"
	AWSSecretMountPathAnnotation = "secrets.k8s.aws/mount-path"
	AWSSecretFileNameAnnotation  = "secrets.k8s.aws/secret-filename"
)

var (
	AWSSecretMountPathPrefix = []string{string(os.PathSeparator), "etc", "flyte", "secrets"}
)

// AWSSecretManagerInjector allows injecting of secrets into pods by specifying annotations on the Pod that either EnvVarSource or SecretVolumeSource in
// the Pod Spec. It'll, by default, mount secrets as files into pods.
// The current version does not allow mounting an entire secret object (with all keys inside it). It only supports mounting
// a single key from the referenced secret object.
// The secret.Group will be used to reference the k8s secret object, the Secret.Key will be used to reference a key inside
// and the secret.Version will be ignored.
// Environment variables will be named _FSEC_<SecretGroup>_<SecretKey>. Files will be mounted on
// /etc/flyte/secrets/<SecretGroup>/<SecretKey>
type AWSSecretManagerInjector struct {
}

func formatAWSSecretArn(secret *core.Secret) string {
	return strings.TrimRight(secret.Group, ":") + ":" + strings.TrimLeft(secret.Key, ":")
}

func formatAWSSecretMount(secret *core.Secret) string {
	return filepath.Join(append(AWSSecretMountPathPrefix, secret.Group)...)
}

func (i AWSSecretManagerInjector) Type() SecretManagerType {
	return SecretManagerTypeAWS
}

func (i AWSSecretManagerInjector) Inject(ctx context.Context, secret *core.Secret, p *corev1.Pod) (newP *corev1.Pod, injected bool, err error) {
	if len(secret.Group) == 0 || len(secret.Key) == 0 {
		return nil, false, fmt.Errorf("k8s Secrets Webhook require both key and group to be set. "+
			"Secret: [%v]", secret)
	}

	switch secret.MountRequirement {
	case core.Secret_ANY:
		fallthrough
	case core.Secret_FILE:
		p.Annotations[AWSSecretArnAnnotation] = formatAWSSecretArn(secret)
		p.Annotations[AWSSecretMountPathAnnotation] = formatAWSSecretMount(secret)
		p.Annotations[AWSSecretFileNameAnnotation] = secret.Key

		// Inject AWS secret-inject webhook annotations to mount the secret in a predictable location.
		envVars := []corev1.EnvVar{
			// Set environment variable to let the container know where to find the mounted files.
			{
				Name:  SecretPathDefaultDirEnvVar,
				Value: filepath.Join(AWSSecretMountPathPrefix...),
			},
			// Sets an empty prefix to let the containers know the file names will match the secret keys as-is.
			{
				Name:  SecretPathFilePrefixEnvVar,
				Value: "",
			},
		}

		for _, envVar := range envVars {
			p.Spec.InitContainers = UpdateEnvVars(p.Spec.InitContainers, envVar)
			p.Spec.Containers = UpdateEnvVars(p.Spec.Containers, envVar)
		}
	case core.Secret_ENV_VAR:
		fallthrough
	default:
		err := fmt.Errorf("unrecognized mount requirement [%v] for secret [%v]", secret.MountRequirement.String(), secret.Key)
		logger.Error(ctx, err)
		return p, false, err
	}

	return p, true, nil
}

func NewAWSSecretManagerInjector() AWSSecretManagerInjector {
	return AWSSecretManagerInjector{}
}
