define go-mod-version
$(shell go mod graph | grep $(1) | head -n 1 | cut -d'@' -f 2)
endef

# Using controller-gen to fetch external CRDs and put them in defined folder folder
# They can be used e.g. in testing using EnvTest where controller under test
# requires additional resources to manage.
#
# $(1) - repository to fetch CRDs from, e.g. github.com/openshift/api
# $(2) - location, e.g. route/v1
# $(3) - target folder
#
# Example use in Makefile target: $(call fetch-external-crds,github.com/openshift/api,route/v1,test/testdata/crds)
define fetch-external-crds
GOFLAGS="-mod=readonly" $(CONTROLLER_GEN) crd \
paths=$(shell go env GOPATH)/pkg/mod/$(1)@$(call go-mod-version,$(1))/$(2)/... \
output:crd:artifacts:config=$(3)
endef
