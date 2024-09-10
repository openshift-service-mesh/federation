# Check unset variables
set -u
# Print commands
set -x

WD=$(dirname "$0")
ROOT=$(pwd)

istio_version=$1
if [ -z "$istio_version" ]; then
  echo "Environment variable 'istio_version' must be specified"
  exit 1
fi

curl -L https://istio.io/downloadIstio | ISTIO_VERSION="$istio_version" sh -
chmod u+x $ROOT/istio-$istio_version/bin/istioctl

mkdir -p "$ROOT/test/testdata/manifests/$istio_version"
$ROOT/istio-$istio_version/bin/istioctl manifest generate -f "$ROOT/test/testdata/istio.yaml" > "$ROOT/test/testdata/manifests/$istio_version/istio.yaml"
rm -rf $ROOT/istio-$istio_version
