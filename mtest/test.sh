#!/bin/sh

TARGET="$1"

sudo -b sh -c "echo \$\$ >/tmp/placemat_pid$$; exec $PLACEMAT output/cluster.yml" >/dev/null 2>&1
sleep 1
PLACEMAT_PID=$(cat /tmp/placemat_pid$$)
echo "placemat PID: $PLACEMAT_PID"

fin() {
    chmod 600 ./mtest_key
    echo "-------- host1: cke log"
    ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -i ./mtest_key cybozu@${HOST1} docker logs cke
    sudo kill $PLACEMAT_PID
    echo "waiting for placemat to terminate..."
    while true; do
        if [ -d /proc/$PLACEMAT_PID ]; then
            sleep 1
            continue
        fi
        break
    done
}
trap fin INT TERM HUP 0

$GINKGO -v -focus="${TARGET}"
RET=$?

exit $RET
