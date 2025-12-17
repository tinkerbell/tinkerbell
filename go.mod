module github.com/tinkerbell/tinkerbell

go 1.24.1

require (
	dario.cat/mergo v1.0.2
	github.com/Masterminds/sprig/v3 v3.3.0
	github.com/avast/retry-go/v4 v4.7.0
	github.com/bmc-toolbox/bmclib/v2 v2.3.5-0.20251010091507-63cb571e5597
	github.com/ccoveille/go-safecast/v2 v2.0.0
	github.com/cenkalti/backoff/v5 v5.0.3
	github.com/containerd/containerd v1.7.29
	github.com/containers/image/v5 v5.36.2
	github.com/diskfs/go-diskfs v1.7.0
	github.com/distribution/reference v0.6.0
	github.com/docker/docker v28.5.2+incompatible
	github.com/fsnotify/fsnotify v1.9.0
	github.com/gin-gonic/gin v1.11.0
	github.com/gliderlabs/ssh v0.3.8
	github.com/go-logr/logr v1.4.3
	github.com/go-playground/validator/v10 v10.28.0
	github.com/golang/mock v1.6.0
	github.com/google/go-cmp v0.7.0
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.1-0.20210315223345-82c243799c99
	github.com/insomniacslk/dhcp v0.0.0-20251020182700-175e84fbb167
	github.com/jacobweinstock/registrar v0.4.7
	github.com/jaypipes/ghw v0.21.2
	github.com/nats-io/nats.go v1.47.0
	github.com/oklog/ulid/v2 v2.1.1
	github.com/opencontainers/runtime-spec v1.2.1
	github.com/peterbourgon/ff/v4 v4.0.0-beta.1
	github.com/pin/tftp/v3 v3.1.0
	github.com/prometheus/client_golang v1.23.2
	github.com/spf13/pflag v1.0.10
	github.com/stretchr/testify v1.11.1
	github.com/tinkerbell/tinkerbell/api v0.0.0 // v0.0.0 is used as a placeholder because a replace directive is used to point to the local api directory
	github.com/vishvananda/netlink v1.3.1
	go.etcd.io/etcd/server/v3 v3.6.6
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.63.0
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.64.0
	go.opentelemetry.io/otel v1.39.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.39.0
	go.opentelemetry.io/otel/sdk v1.39.0
	go.opentelemetry.io/otel/trace v1.39.0
	go.uber.org/zap v1.27.1
	golang.org/x/crypto v0.46.0
	golang.org/x/net v0.47.0
	golang.org/x/sync v0.19.0
	golang.org/x/sys v0.39.0
	golang.org/x/text v0.32.0
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251202230838-ff82c1b0f217
	google.golang.org/grpc v1.77.0
	google.golang.org/protobuf v1.36.10
	gopkg.in/yaml.v3 v3.0.1
	k8s.io/api v0.33.4
	k8s.io/apiextensions-apiserver v0.33.4
	k8s.io/apimachinery v0.33.4
	k8s.io/client-go v0.33.4
	k8s.io/component-base v0.33.4
	k8s.io/klog/v2 v2.130.1
	k8s.io/kubernetes v1.33.4
	quamina.net/go/quamina v1.5.2-0.20250207005432-0526acc321a8
	sigs.k8s.io/controller-runtime v0.21.0
)

require (
	cel.dev/expr v0.24.0 // indirect
	cyphar.com/go-pathrs v0.2.1 // indirect
	github.com/AdaLogics/go-fuzz-headers v0.0.0-20240806141605-e8a1dd7889d6 // indirect
	github.com/AdamKorcz/go-118-fuzz-build v0.0.0-20230306123547-8075edf89bb0 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20250102033503-faa5f7b0171c // indirect
	github.com/BurntSushi/toml v1.5.0 // indirect
	github.com/JeffAshton/win_pdh v0.0.0-20161109143554-76bb4ee9f0ab // indirect
	github.com/Jeffail/gabs/v2 v2.7.0 // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/semver/v3 v3.3.0 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/Microsoft/hcsshim v0.13.0 // indirect
	github.com/NYTimes/gziphandler v1.1.1 // indirect
	github.com/VictorLowther/simplexml v0.0.0-20180716164440-0bff93621230 // indirect
	github.com/VictorLowther/soap v0.0.0-20150314151524-8e36fca84b22 // indirect
	github.com/anchore/go-lzo v0.1.0 // indirect
	github.com/anmitsu/go-shlex v0.0.0-20200514113438-38f4b401e2be // indirect
	github.com/antlr4-go/antlr/v4 v4.13.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/bmc-toolbox/common v0.0.0-20250112191656-b6de52e8303d // indirect
	github.com/bytedance/sonic v1.14.0 // indirect
	github.com/bytedance/sonic/loader v0.3.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/chzyer/readline v1.5.1 // indirect
	github.com/cloudwego/base64x v0.1.6 // indirect
	github.com/container-storage-interface/spec v1.9.0 // indirect
	github.com/containerd/cgroups/v3 v3.0.5 // indirect
	github.com/containerd/containerd/api v1.8.0 // indirect
	github.com/containerd/continuity v0.4.4 // indirect
	github.com/containerd/errdefs v1.0.0 // indirect
	github.com/containerd/errdefs/pkg v0.3.0 // indirect
	github.com/containerd/fifo v1.1.0 // indirect
	github.com/containerd/log v0.1.0 // indirect
	github.com/containerd/platforms v0.2.1 // indirect
	github.com/containerd/ttrpc v1.2.7 // indirect
	github.com/containerd/typeurl/v2 v2.2.3 // indirect
	github.com/containers/storage v1.59.1 // indirect
	github.com/coreos/go-oidc v2.3.0+incompatible // indirect
	github.com/coreos/go-semver v0.3.1 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/cyphar/filepath-securejoin v0.6.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/djherbis/times v1.6.0 // indirect
	github.com/docker/go-connections v0.5.0 // indirect
	github.com/docker/go-events v0.0.0-20190806004212-e31b211e4f1c // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/elliotwutingfeng/asciiset v0.0.0-20230602022725-51bbb787efab // indirect
	github.com/emicklei/go-restful/v3 v3.12.1 // indirect
	github.com/euank/go-kmsg-parser v2.0.0+incompatible // indirect
	github.com/evanphx/json-patch v5.9.0+incompatible // indirect
	github.com/evanphx/json-patch/v5 v5.9.11 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fxamacker/cbor/v2 v2.7.0 // indirect
	github.com/gabriel-vasile/mimetype v1.4.10 // indirect
	github.com/ghodss/yaml v1.0.0 // indirect
	github.com/gin-contrib/sse v1.1.0 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-logr/zapr v1.3.0 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/go-openapi/jsonpointer v0.21.0 // indirect
	github.com/go-openapi/jsonreference v0.21.0 // indirect
	github.com/go-openapi/swag v0.23.1 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-test/deep v1.1.1 // indirect
	github.com/goccy/go-json v0.10.2 // indirect
	github.com/goccy/go-yaml v1.18.0 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang-jwt/jwt/v5 v5.2.2 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/btree v1.1.3 // indirect
	github.com/google/cadvisor v0.52.1 // indirect
	github.com/google/cel-go v0.23.2 // indirect
	github.com/google/gnostic-models v0.6.9 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/websocket v1.5.4-0.20250319132907-e064f32e3674 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus v1.0.1 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware/v2 v2.1.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.27.3 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/huandu/xstrings v1.5.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jacobweinstock/iamt v0.0.0-20230502042727-d7cdbe67d9ef // indirect
	github.com/jaypipes/pcidb v1.1.1 // indirect
	github.com/jonboulle/clockwork v0.5.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/josharian/native v1.1.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/karrick/godirwalk v1.17.0 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/libopenstorage/openstorage v1.0.0 // indirect
	github.com/mailru/easyjson v0.9.0 // indirect
	github.com/manifoldco/promptui v0.9.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mdlayher/packet v1.1.2 // indirect
	github.com/mdlayher/socket v0.4.1 // indirect
	github.com/mistifyio/go-zfs v2.1.2-0.20190413222219-f784269be439+incompatible // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/moby/locker v1.0.1 // indirect
	github.com/moby/spdystream v0.5.0 // indirect
	github.com/moby/sys/capability v0.4.0 // indirect
	github.com/moby/sys/mountinfo v0.7.2 // indirect
	github.com/moby/sys/sequential v0.6.0 // indirect
	github.com/moby/sys/signal v0.7.0 // indirect
	github.com/moby/sys/user v0.4.0 // indirect
	github.com/moby/sys/userns v0.1.0 // indirect
	github.com/moby/term v0.5.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f // indirect
	github.com/nats-io/nkeys v0.4.11 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/opencontainers/cgroups v0.0.1 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	github.com/opencontainers/selinux v1.13.0 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/pierrec/lz4/v4 v4.1.17 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pkg/xattr v0.4.9 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/pquerna/cachecontrol v0.1.0 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.66.1 // indirect
	github.com/prometheus/procfs v0.16.1 // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	github.com/quic-go/quic-go v0.57.0 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/satori/go.uuid v1.2.0 // indirect
	github.com/shopspring/decimal v1.4.0 // indirect
	github.com/sirupsen/logrus v1.9.4-0.20230606125235-dd1b4c2e81af // indirect
	github.com/soheilhy/cmux v0.1.5 // indirect
	github.com/spf13/cast v1.7.0 // indirect
	github.com/spf13/cobra v1.9.1 // indirect
	github.com/stmcginnis/gofish v0.20.0 // indirect
	github.com/stoewer/go-strcase v1.3.0 // indirect
	github.com/tmc/grpc-websocket-proxy v0.0.0-20220101234140-673ab2c3ae75 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	github.com/u-root/uio v0.0.0-20240224005618-d2acac8f3701 // indirect
	github.com/ugorji/go/codec v1.3.0 // indirect
	github.com/ulikunitz/xz v0.5.14 // indirect
	github.com/vishvananda/netns v0.0.5 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/xiang90/probing v0.0.0-20221125231312-a49e3df8f510 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.etcd.io/bbolt v1.4.3 // indirect
	go.etcd.io/etcd/api/v3 v3.6.6 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.6.6 // indirect
	go.etcd.io/etcd/client/v3 v3.6.6 // indirect
	go.etcd.io/etcd/pkg/v3 v3.6.6 // indirect
	go.etcd.io/raft/v3 v3.6.0 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.39.0 // indirect
	go.opentelemetry.io/otel/metric v1.39.0 // indirect
	go.opentelemetry.io/proto/otlp v1.9.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.yaml.in/yaml/v2 v2.4.2 // indirect
	golang.org/x/arch v0.20.0 // indirect
	golang.org/x/exp v0.0.0-20241217172543-b2144cdd0a67 // indirect
	golang.org/x/oauth2 v0.32.0 // indirect
	golang.org/x/term v0.38.0 // indirect
	golang.org/x/time v0.12.0 // indirect
	golang.org/x/tools v0.39.0 // indirect
	gomodules.xyz/jsonpatch/v2 v2.4.0 // indirect
	google.golang.org/genproto v0.0.0-20241118233622-e639e219e697 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20251202230838-ff82c1b0f217 // indirect
	gopkg.in/evanphx/json-patch.v4 v4.12.0 // indirect
	gopkg.in/go-jose/go-jose.v2 v2.6.3 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	howett.net/plist v1.0.2-0.20250314012144-ee69052608d9 // indirect
	k8s.io/apiserver v0.33.4 // indirect
	k8s.io/cloud-provider v0.33.4 // indirect
	k8s.io/cluster-bootstrap v0.0.0 // indirect
	k8s.io/component-helpers v0.33.4 // indirect
	k8s.io/controller-manager v0.33.4 // indirect
	k8s.io/cri-api v0.33.4 // indirect
	k8s.io/cri-client v0.0.0 // indirect
	k8s.io/csi-translation-lib v0.0.0 // indirect
	k8s.io/dynamic-resource-allocation v0.0.0 // indirect
	k8s.io/endpointslice v0.32.0 // indirect
	k8s.io/externaljwt v0.0.0 // indirect
	k8s.io/kms v0.33.4 // indirect
	k8s.io/kube-aggregator v0.32.4 // indirect
	k8s.io/kube-controller-manager v0.0.0 // indirect
	k8s.io/kube-openapi v0.0.0-20250318190949-c8a335a9a2ff // indirect
	k8s.io/kube-scheduler v0.0.0 // indirect
	k8s.io/kubectl v0.32.4 // indirect
	k8s.io/kubelet v0.33.4 // indirect
	k8s.io/metrics v0.33.4 // indirect
	k8s.io/mount-utils v0.0.0 // indirect
	k8s.io/pod-security-admission v0.0.0 // indirect
	k8s.io/utils v0.0.0-20241210054802-24370beab758 // indirect
	sigs.k8s.io/apiserver-network-proxy/konnectivity-client v0.31.2 // indirect
	sigs.k8s.io/json v0.0.0-20241010143419-9aa6b5e7a4b3 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.6.0 // indirect
	sigs.k8s.io/yaml v1.4.0 // indirect
)

replace (
	github.com/tinkerbell/tinkerbell/api => ./api
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc => go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.60.0
	k8s.io/api => k8s.io/api v0.33.4
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.33.4
	k8s.io/apimachinery => k8s.io/apimachinery v0.33.4
	k8s.io/apiserver => k8s.io/apiserver v0.33.4
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.33.4
	k8s.io/client-go => k8s.io/client-go v0.33.4
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.33.4
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.33.4
	k8s.io/code-generator => k8s.io/code-generator v0.33.4
	k8s.io/component-base => k8s.io/component-base v0.33.4
	k8s.io/component-helpers => k8s.io/component-helpers v0.33.4
	k8s.io/controller-manager => k8s.io/controller-manager v0.33.4
	k8s.io/cri-api => k8s.io/cri-api v0.33.4
	k8s.io/cri-client => k8s.io/cri-client v0.33.4
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.33.4
	k8s.io/dynamic-resource-allocation => k8s.io/dynamic-resource-allocation v0.33.4
	k8s.io/endpointslice => k8s.io/endpointslice v0.33.4
	k8s.io/externaljwt => k8s.io/externaljwt v0.33.4
	k8s.io/kms => k8s.io/kms v0.33.4
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.33.4
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.33.4
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.33.4
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.33.4
	k8s.io/kubectl => k8s.io/kubectl v0.33.4
	k8s.io/kubelet => k8s.io/kubelet v0.33.4
	k8s.io/metrics => k8s.io/metrics v0.33.4
	k8s.io/mount-utils => k8s.io/mount-utils v0.33.4
	k8s.io/pod-security-admission => k8s.io/pod-security-admission v0.33.4
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.33.4
	k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.33.4
	k8s.io/sample-controller => k8s.io/sample-controller v0.33.4
)
