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

for region in east west
do
  mkdir -p "$ROOT/test/testdata/manifests/$istio_version"
  sed "s/{{.clusterName}}/$region/g" "$ROOT/test/testdata/istio.yaml" > "$ROOT/test/testdata/manifests/$istio_version/istio-operator-$region.yaml"
  $ROOT/istio-$istio_version/bin/istioctl manifest generate -f "$ROOT/test/testdata/manifests/$istio_version/istio-operator-$region.yaml" > "$ROOT/test/testdata/manifests/$istio_version/istio-$region.yaml"
  rm "$ROOT/test/testdata/manifests/$istio_version/istio-operator-$region.yaml"
done
rm -rf $ROOT/istio-$istio_version
