#!/bin/bash

# example: ROOK_IMAGE=build-e858f56d/ceph-amd64:latest NOOBAA_IMAGE=noobaa/noobaa-operator:1.1.0 OCS_IMAGE=placeholder CSV_VERSION=1.1.1 hack/generate-manifests.sh

set -e

source hack/common.sh
source hack/operator-sdk-common.sh

function help_txt() {
	echo "Environment Variables"
	echo "    NOOBAA_IMAGE:         (required) The noobaa operator container image to integrate with"
	echo "    ROOK_IMAGE:           (required) The rook operator container image to integrate with"
	echo ""
	echo "Example usage:"
	echo "    NOOBAA_IMAGE=<image> ROOK_IMAGE=<image> $0"
}

# check required env vars
if [ -z "$NOOBAA_IMAGE" ] || [ -z "$ROOK_IMAGE" ]; then
	help_txt
	echo ""
	echo "ERROR: Missing required environment variables"
	exit 1
fi

# always start fresh and remove any previous artifacts that may exist.
rm -rf "$(dirname $OCS_FINAL_DIR)"
mkdir -p "$(dirname $OCS_FINAL_DIR)"
rm -rf $OUTDIR_TEMPLATES
mkdir -p $OUTDIR_TEMPLATES
mkdir -p $OUTDIR_CRDS
mkdir -p $OUTDIR_TOOLS

# ==== DUMP NOOBAA YAMLS ====
noobaa_dump_crds_cmd="crd yaml"
noobaa_dump_csv_cmd="olm csv yaml"
echo "Dumping Noobaa csv using command: $IMAGE_RUN_CMD $NOOBAA_IMAGE $noobaa_dump_csv_cmd"
($IMAGE_RUN_CMD "$NOOBAA_IMAGE" "$noobaa_dump_csv_cmd") > $NOOBAA_CSV
echo "Dumping Noobaa crds using command: $IMAGE_RUN_CMD $NOOBAA_IMAGE $noobaa_dump_crds_cmd"
($IMAGE_RUN_CMD "$NOOBAA_IMAGE" "$noobaa_dump_crds_cmd") > $OUTDIR_CRDS/noobaa-crd.yaml

# ==== DUMP ROOK YAMLS ====
rook_template_dir="/etc/ceph-csv-templates"
rook_csv_template="rook-ceph-ocp.vVERSION.clusterserviceversion.yaml.in"
rook_crds_dir=$rook_template_dir/crds
crd_list=$(mktemp)
echo "Dumping rook csv using command: $IMAGE_RUN_CMD --entrypoint=cat $ROOK_IMAGE $rook_template_dir/$rook_csv_template"
$IMAGE_RUN_CMD --entrypoint=cat "$ROOK_IMAGE" $rook_template_dir/$rook_csv_template > $ROOK_CSV
echo "Listing rook crds using command: $IMAGE_RUN_CMD --entrypoint=ls $ROOK_IMAGE -1 $rook_crds_dir/"
$IMAGE_RUN_CMD --entrypoint=ls "$ROOK_IMAGE" -1 $rook_crds_dir/ > "$crd_list"
# shellcheck disable=SC2013
for i in $(cat "$crd_list"); do
        # shellcheck disable=SC2059
	crd_file=$(printf ${rook_crds_dir}/"$i" | tr -d '[:space:]')
	echo "Dumping rook crd $crd_file using command: $IMAGE_RUN_CMD --entrypoint=cat $ROOK_IMAGE $crd_file"
	($IMAGE_RUN_CMD --entrypoint=cat "$ROOK_IMAGE" "$crd_file") > $OUTDIR_CRDS/"$(basename "$crd_file")"
done;
rm -f "$crd_list"

# ==== DUMP OCS YAMLS ====
# Generate an OCS CSV using the operator-sdk.
# This is the base CSV everything else gets merged into later on.
gen_args="generate kustomize manifests -q"
# shellcheck disable=SC2086
$OPERATOR_SDK $gen_args
pushd config/manager
$KUSTOMIZE edit set image ocs-dev/ocs-operator="$OPERATOR_FULL_IMAGE_NAME"
popd
$KUSTOMIZE build config/manifests | $OPERATOR_SDK generate bundle -q --overwrite=false --version "$CSV_VERSION"
mv "$GOPATH"/src/github.com/openshift/ocs-operator/bundle/manifests/*clusterserviceversion.yaml $OCS_CSV
cp config/crd/bases/* $OUTDIR_CRDS/

echo "Manifests sourced into $OUTDIR_TEMPLATES directory"

mv "$GOPATH"/src/github.com/openshift/ocs-operator/bundle/manifests $OCS_FINAL_DIR
mv "$GOPATH"/src/github.com/openshift/ocs-operator/bundle/metadata "$(dirname $OCS_FINAL_DIR)"/metadata
rm -rf "$GOPATH"/src/github.com/openshift/ocs-operator/bundle
rm "$GOPATH"/src/github.com/openshift/ocs-operator/bundle.Dockerfile
