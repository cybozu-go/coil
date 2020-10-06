package sub

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"time"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	"github.com/cybozu-go/coil/v2/pkg/constants"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var interval time.Duration

var replaceCmd = &cobra.Command{
	Use:   "replace",
	Short: "remove pods run by Coil v1, then delete reserved blocks",
	Long: `This command finalizes the migration from v1 to v2 by deleting
all the currently running Pods and then deleting reserved blocks.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true

		fmt.Fprintf(os.Stderr, "DO YOU WANT TO PROCEED? [y/N] ")
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		switch scanner.Text() {
		case "y", "Y", "YES", "yes":
		default:
			fmt.Fprintln(os.Stderr, "Aborted.")
			return nil
		}
		return doReplace(context.Background())
	},
}

func doReplace(ctx context.Context) error {
	k8sClient, err := getClient()
	if err != nil {
		return err
	}

	pods := &corev1.PodList{}
	if err := k8sClient.List(ctx, pods); err != nil {
		return fmt.Errorf("failed to list all pods: %w", err)
	}

	podMap := make(map[string][]*corev1.Pod)
	for i := range pods.Items {
		pod := &(pods.Items[i])
		if pod.Spec.HostNetwork {
			continue
		}
		if pod.Spec.NodeName == "" {
			continue
		}

		podMap[pod.Spec.NodeName] = append(podMap[pod.Spec.NodeName], pod)
	}

	var failures []string

	for nodeName, pl := range podMap {
		fmt.Printf("Deleting pods on node %s...\n", nodeName)

	OUTER:
		for _, pod := range pl {
			err := k8sClient.Delete(ctx, pod)
			if apierrors.IsNotFound(err) {
				continue
			}
			if err != nil {
				fmt.Printf("  failed to delete %s/%s, continuing...\n", pod.Namespace, pod.Name)
				failures = append(failures, fmt.Sprintf("%s/%s", pod.Namespace, pod.Name))
				continue
			}
			fmt.Printf("  deleting %s/%s\n", pod.Namespace, pod.Name)

			for i := 0; i < 6000; i++ {
				time.Sleep(100 * time.Millisecond)
				p2 := &corev1.Pod{}
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: pod.Namespace, Name: pod.Name}, p2)
				if apierrors.IsNotFound(err) {
					continue OUTER
				}
				// ignore other errors as they may be temporary
			}

			fmt.Printf("  timed out to delete %s/%s, continuing...\n", pod.Namespace, pod.Name)
			failures = append(failures, fmt.Sprintf("%s/%s", pod.Namespace, pod.Name))
		}

		time.Sleep(interval)
	}

	if len(failures) > 0 {
		fmt.Println("saving pod names that could not be deleted in /tmp/pod-delete-failure.txt")
		f, err := os.Create("/tmp/pod-delete-failure.txt")
		if err != nil {
			fmt.Println("failed to save")
		} else {
			defer f.Close()

			for _, name := range failures {
				fmt.Fprintln(f, name)
			}
			f.Sync()
		}
	}

	fmt.Println("deleting reserved address blocks")
	err = k8sClient.DeleteAllOf(ctx, &coilv2.AddressBlock{}, client.MatchingLabels{
		constants.LabelReserved: "true",
	})
	return err
}

func init() {
	rootCmd.AddCommand(replaceCmd)
	replaceCmd.Flags().DurationVar(&interval, "interval", 10*time.Second, "interval before starting to remove pods on the next node")
}
