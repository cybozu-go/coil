package sub

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	coilv1 "github.com/cybozu-go/coil"
	"github.com/cybozu-go/coil/model"
	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	"github.com/cybozu-go/coil/v2/pkg/constants"
	"github.com/cybozu-go/etcdutil"
	"github.com/cybozu-go/netutil"
	"github.com/spf13/cobra"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8sjson "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var etcdcfg = coilv1.NewEtcdConfig()

var skipUninstall bool

type k8sObject interface {
	metav1.Object
	runtime.Object
}

var v1Resources = []k8sObject{
	&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceSystem, Name: "coil-node"}},
	&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceSystem, Name: "coil-controllers"}},
	&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceSystem, Name: "coil-config"}},
	&rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "coil-node"}},
	&rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "coil-controllers"}},
	&rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "coil-node"}},
	&rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "coil-controllers"}},
	&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceSystem, Name: "coil-node"}},
	&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceSystem, Name: "coil-controller"}},
}

var dumpCmd = &cobra.Command{
	Use:   "dump",
	Short: "uninstall Coil v1 and convert its data into v2",
	Long: `This command does the followings:

- Remove Coil v1 resources from the cluster.
- Annotate namespaces using non-default address pools.
- Convert v1 data into v2 and dump them as YAML.

These steps are idempotent and can be run multiple times.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		return dump(context.Background())
	},
}

func dump(ctx context.Context) error {
	etcd, err := etcdutil.NewClient(etcdcfg)
	if err != nil {
		return err
	}
	defer etcd.Close()

	k8sClient, err := getClient()
	if err != nil {
		return err
	}

	fmt.Fprintln(os.Stderr, "uninstalling Coil v1 ...")
OUTER:
	for _, obj := range v1Resources {
		if skipUninstall {
			fmt.Fprintln(os.Stderr, "  skip")
			break
		}
		err = k8sClient.Delete(ctx, obj, client.PropagationPolicy(metav1.DeletePropagationForeground))
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete %T %s: %w", obj, obj.GetName(), err)
		}

		for i := 0; i < 100; i++ {
			time.Sleep(1 * time.Second)
			t := obj.DeepCopyObject().(k8sObject)
			err = k8sClient.Get(ctx, client.ObjectKey{Namespace: obj.GetNamespace(), Name: obj.GetName()}, t)
			if err == nil {
				continue
			}
			if apierrors.IsNotFound(err) {
				fmt.Fprintf(os.Stderr, "  deleted %T %s\n", obj, obj.GetName())
				continue OUTER
			}
			return fmt.Errorf("failed to check %T %s: %w", obj, obj.GetName(), err)
		}

		return fmt.Errorf("timed out to delete %T %s", obj, obj.GetName())
	}

	v1Model := model.NewEtcdModel(etcd)
	pools, err := v1Model.ListPools(ctx)
	if err != nil {
		return fmt.Errorf("failed to list pools: %w", err)
	}

	v2Pools := convertPools(pools)

	for name := range v2Pools {
		if name == "default" {
			continue
		}

		ns := &corev1.Namespace{}
		err := k8sClient.Get(ctx, client.ObjectKey{Name: name}, ns)
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("failed to get namespace %s: %w", name, err)
		}

		if ns.Annotations == nil {
			ns.Annotations = make(map[string]string)
		}
		ns.Annotations[constants.AnnPool] = name

		err = k8sClient.Update(ctx, ns)
		if err != nil {
			return fmt.Errorf("failed to annotate namespace %s: %w", name, err)
		}

		fmt.Fprintf(os.Stderr, "annotated namespace %s\n", name)
	}

	nodes := &corev1.NodeList{}
	err = k8sClient.List(ctx, nodes)
	if err != nil {
		return fmt.Errorf("failed to list nodes: %w", err)
	}

	var v2Blocks []*coilv2.AddressBlock
	for _, node := range nodes.Items {
		blocks, err := v1Model.GetMyBlocks(ctx, node.Name)
		if err != nil {
			return fmt.Errorf("failed to get blocks for %s: %w", node.Name, err)
		}
		if len(blocks) == 0 {
			continue
		}

		t, err := convertBlocks(blocks, node.Name, v2Pools)
		if err != nil {
			return err
		}
		v2Blocks = append(v2Blocks, t...)
	}

	dumpYAML(os.Stdout, v2Pools, v2Blocks)

	return nil
}

func convertPools(pools map[string]*coilv1.AddressPool) map[string]*coilv2.AddressPool {
	r := make(map[string]*coilv2.AddressPool)
	for name, v1p := range pools {
		v2p := &coilv2.AddressPool{}
		v2p.Name = name
		v2p.Spec.BlockSizeBits = int32(v1p.BlockSize)
		for _, n := range v1p.Subnets {
			ns := n.String()
			v2p.Spec.Subnets = append(v2p.Spec.Subnets, coilv2.SubnetSet{IPv4: &ns})
		}
		r[name] = v2p
	}
	return r
}

func convertBlocks(blocks map[string][]*net.IPNet, nodeName string, pools map[string]*coilv2.AddressPool) ([]*coilv2.AddressBlock, error) {
	var v2Blocks []*coilv2.AddressBlock
	for k, v := range blocks {
		pool := pools[k]
		if pool == nil {
			fmt.Fprintf(os.Stderr, "skipping a reserved block from non-existent pool %s for node %s\n", k, nodeName)
			continue
		}
		bl, err := convertBlocksForPool(v, nodeName, pool)
		if err != nil {
			return nil, err
		}
		v2Blocks = append(v2Blocks, bl...)
	}
	return v2Blocks, nil
}

func convertBlocksForPool(blocks []*net.IPNet, nodeName string, pool *coilv2.AddressPool) ([]*coilv2.AddressBlock, error) {
	var v2Blocks []*coilv2.AddressBlock
	for _, b := range blocks {
		v2b, err := convertBlock(b, nodeName, pool)
		if err != nil {
			return nil, err
		}
		v2Blocks = append(v2Blocks, v2b)
	}
	return v2Blocks, nil
}

func convertBlock(n *net.IPNet, nodeName string, pool *coilv2.AddressPool) (*coilv2.AddressBlock, error) {
	var index int64
	blockSize := int64(1) << pool.Spec.BlockSizeBits
	for _, sub := range pool.Spec.Subnets {
		_, subn, _ := net.ParseCIDR(*sub.IPv4)
		if subn.Contains(n.IP) {
			diff := netutil.IPDiff(subn.IP, n.IP)
			index += (diff / blockSize)

			block := &coilv2.AddressBlock{}
			block.Name = fmt.Sprintf("%s-%d-v1", pool.Name, index)
			block.Labels = map[string]string{
				constants.LabelPool:     pool.Name,
				constants.LabelNode:     nodeName,
				constants.LabelReserved: "true",
			}
			block.Index = int32(index)
			ns := n.String()
			block.IPv4 = &ns
			return block, nil
		}

		ones, bits := subn.Mask.Size()
		index += (int64(1) << (bits - ones)) / blockSize
	}

	return nil, fmt.Errorf("block %s cannot exist in pool %s", n.String(), pool.Name)
}

func dumpYAML(w io.Writer, pools map[string]*coilv2.AddressPool, blocks []*coilv2.AddressBlock) {
	w = k8sjson.YAMLFramer.NewFrameWriter(w)

	s := k8sjson.NewSerializerWithOptions(k8sjson.DefaultMetaFactory, scheme, scheme, k8sjson.SerializerOptions{
		Yaml: true,
	})

	for _, p := range pools {
		un := &unstructured.Unstructured{}
		if err := scheme.Convert(p, un, nil); err != nil {
			panic(fmt.Errorf("failed to convert AddressPool into Unstructured: %w", err))
		}
		s.Encode(un, w)
	}
	for _, b := range blocks {
		un := &unstructured.Unstructured{}
		if err := scheme.Convert(b, un, nil); err != nil {
			panic(fmt.Errorf("failed to convert AddressBlock into Unstructured: %w", err))
		}
		s.Encode(un, w)
	}
}

func init() {
	rootCmd.AddCommand(dumpCmd)
	dumpCmd.Flags().BoolVar(&skipUninstall, "skip-uninstall", false, "DANGER!! do not uninstall Coil v1")
	etcdcfg.AddPFlags(dumpCmd.Flags())
}
