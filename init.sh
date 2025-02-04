#!/bin/bash

usage() {
    echo "Usage: $0 --vm-name <vm-name> --namespace <namespace> --src-kubeconfig <file> --dst-kubeconfig <file> [--preserve-pod-ip] [--help]"
    echo
    echo "Options:"
    echo "  --vm-name           Virtual machine name (required)"
    echo "  --namespace         Namespace to work on (required)"
    echo "  --src-kubeconfig    Source kubeconfig file path (required)"
    echo "  --dst-kubeconfig    Destination kubeconfig file path (required)"
    echo "  --preserve-pod-ip   Preserve pod IP address during migration (optional)"
    echo "  --help              Display this help message and exit"
    exit 1
}

VM_NAME=""
NAMESPACE=""
SRC_KUBECONFIG=""
DST_KUBECONFIG=""
VERBOSE=0
PVC_NAME=""
DST_HOST_IP=""
DST_NODE_PORT=""
PRESERVE_POD_IP=0

while [[ $# -gt 0 ]]; do
    case "$1" in
        --vm-name)
            VM_NAME="$2"
            export VM_NAME
            shift 2
            ;;
        --namespace)
            NAMESPACE="$2"
            export NAMESPACE
            shift 2
            ;;
        --src-kubeconfig)
            SRC_KUBECONFIG="$2"
            export SRC_KUBECONFIG
            shift 2
            ;;
        --dst-kubeconfig)
            DST_KUBECONFIG="$2"
            export DST_KUBECONFIG
            shift 2
            ;;
        --help)
            usage
            ;;
        *)
            echo "Unknown option: $1"
            usage
            ;;
    esac
done

if [[ -z "$VM_NAME" || -z "$NAMESPACE" || -z "$SRC_KUBECONFIG" || -z "$DST_KUBECONFIG" ]]; then
    echo "Error: --vm-name, --namespace, --src-kubeconfig, and --dst-kubeconfig are required."
    usage
else
    echo "Checking source VM status"
    src_vm_state=`oc get vm $VM_NAME -n $NAMESPACE --kubeconfig $SRC_KUBECONFIG --no-headers | awk '{print $3}'`
    if [[ $? -eq 0 ]]; then echo $src_vm_state; else echo "No Running VM" ; fi
    
    echo "Checking destination VM status"
    dst_vm_state=`oc get vm $VM_NAME -n $NAMESPACE --kubeconfig $DST_KUBECONFIG --no-headers | awk '{print $3}'`
    if [[ $? -eq 0 ]]; then echo $dst_vm_state; else echo "No Running VM"; fi

    if [[ $dst_vm_state != "Stopped" ]]; then
        if [[ $dst_vm_state == "" ]]; then
            echo "Exporting VM from source cluster"
            oc get vm $VM_NAME -n $NAMESPACE --kubeconfig $SRC_KUBECONFIG -o yaml > $VM_NAME-vm.yaml
            if [[ $PRESERVE_POD_IP -eq 1 ]]; then
                POD_IP=`oc get vmi $VM_NAME -n $NAMESPACE --kubeconfig $SRC_KUBECONFIG -o=jsonpath='{.status.interfaces[0].ipAddress}'`"/23"
                POD_MAC=`oc get vmi $VM_NAME -n $NAMESPACE --kubeconfig $SRC_KUBECONFIG -o=jsonpath='{.status.interfaces[0].mac}'`
                export ip_annotation="'{\"default\":{\"ip_address\":\"$POD_IP \",\"mac_address\":\"$POD_MAC\"}}'"
                yq e -i '.spec.template.metadata.annotations["k8s.ovn.org/pod-networks"] = env(ip_annotation)' $VM_NAME-vm.yaml
            fi
            yq e -i '.spec.running = false' $VM_NAME-vm.yaml
            oc apply --wait -n $NAMESPACE --kubeconfig $DST_KUBECONFIG -f $VM_NAME-vm.yaml
            echo "Waiting for the destination VM to be created ...... "
            while [[ $( oc get vm $VM_NAME -n $NAMESPACE --kubeconfig $DST_KUBECONFIG --no-headers | awk '{print $3}') != "Stopped"  ]]
            do
                printf  "#"
                sleep 5
            done
        fi
        if [[ $dst_vm_state == "Running" ]]; then
            virtctl stop $VM_NAME --kubeconfig $DST_KUBECONFIG
        fi
    fi
    
    echo "Checking source Replicator"
    src_repl_state=`oc get po $VM_NAME-src-replicator -n $NAMESPACE --kubeconfig $SRC_KUBECONFIG --no-headers | awk '{print $3}' | grep -v "NotFound"`
    if [[ $? -eq 0 ]]; then echo $src_repl_state; else echo "No Running Replicator" ; fi
    
    echo "Checking destination Replicator"
    dst_repl_state=`oc get po $VM_NAME-dst-replicator -n $NAMESPACE --kubeconfig $DST_KUBECONFIG --no-headers | awk '{print $3}' | grep -v "NotFound"`
    if [[ $? -eq 0 ]]; then echo $dst_repl_state; else echo "No Running Replicator" ; fi

    if [[ $src_repl_state != "Running" ]]; then 
        echo "Creating source Replicator"
        yq -i '.metadata.name = strenv(VM_NAME)+"-src-replicator"' manifests/src-repl.yaml
        yq e -i '.metadata.labels.app = env(VM_NAME)+"-src-replicator"' manifests/src-repl.yaml
        yq e -i '.spec.volumes[0].persistentVolumeClaim.claimName = env(VM_NAME)' manifests/src-repl.yaml
        oc apply --wait -n $NAMESPACE --kubeconfig $SRC_KUBECONFIG -f manifests/src-repl.yaml 
        oc wait pod $VM_NAME-src-replicator -n $NAMESPACE --kubeconfig $SRC_KUBECONFIG --for=condition=Ready --timeout=-1m
        echo "Generating source replicator SSH key"
        oc exec $VM_NAME-src-replicator -ti -n $NAMESPACE --kubeconfig $SRC_KUBECONFIG -- bash -c "ssh-keygen -t rsa -b 4096 -N '' -f ~/.ssh/id_rsa"
        echo "Generating source SSH secret"
        oc cp $VM_NAME-src-replicator:/root/.ssh/id_rsa id_rsa -n $NAMESPACE --kubeconfig $SRC_KUBECONFIG
        oc cp $VM_NAME-src-replicator:/root/.ssh/id_rsa.pub id_rsa.pub -n $NAMESPACE --kubeconfig $SRC_KUBECONFIG
        oc create secret generic $VM_NAME-repl-ssh-keys --from-file=id_rsa --from-file=id_rsa.pub -n $NAMESPACE --kubeconfig $SRC_KUBECONFIG
        src_repl_state=`oc get po $VM_NAME-src-replicator -n $NAMESPACE --kubeconfig $SRC_KUBECONFIG --no-headers | awk '{print $3}' | grep -v "NotFound"`
    fi

    if [[ $dst_repl_state != "Running" ]]; then 
        echo "Creating destination Replicator"
        yq e -i '.metadata.name = env(VM_NAME)+"-dst-replicator"' manifests/dst-repl.yaml
        yq e -i '.metadata.labels.app = env(VM_NAME)+"-dst-replicator"' manifests/dst-repl.yaml
        yq e -i '.spec.volumes[0].persistentVolumeClaim.claimName = env(VM_NAME)' manifests/dst-repl.yaml
        oc apply --wait -n $NAMESPACE --kubeconfig $DST_KUBECONFIG -f manifests/dst-repl.yaml
        yq e -i '.metadata.name = env(VM_NAME)+"-dst-svc"' manifests/dst-repl-svc.yaml
        yq e -i '.metadata.labels.app = env(VM_NAME)+"-dst-replicator"' manifests/dst-repl-svc.yaml
        yq e -i '.spec.selector.app = env(VM_NAME)+"-dst-replicator"' manifests/dst-repl-svc.yaml
        oc wait pod $VM_NAME-dst-replicator -n $NAMESPACE --kubeconfig $DST_KUBECONFIG --for=condition=Ready --timeout=-1m
        oc apply -n $NAMESPACE --kubeconfig $DST_KUBECONFIG -f manifests/dst-repl-svc.yaml
        src_ssh_key=`oc exec $VM_NAME-src-replicator -ti -n $NAMESPACE --kubeconfig $SRC_KUBECONFIG -- bash -c "cat ~/.ssh/id_rsa.pub"`
        oc exec $VM_NAME-dst-replicator -ti -n $NAMESPACE --kubeconfig $DST_KUBECONFIG -- bash -c "mkdir ~/.ssh; echo '$src_ssh_key' > ~/.ssh/authorized_keys; chmod 600 ~/.ssh/authorized_keys"
        dst_repl_state=`oc get po $VM_NAME-dst-replicator -n $NAMESPACE --kubeconfig $DST_KUBECONFIG --no-headers | awk '{print $3}' | grep -v "NotFound"`
    fi

    if [ $src_repl_state == "Running" -a $dst_repl_state == "Running" ]; then 
        echo "Getting destination NodePort"
        dst_node_port=`oc get svc $VM_NAME-dst-svc -n $NAMESPACE --kubeconfig $DST_KUBECONFIG -o=jsonpath='{.spec.ports[0].nodePort}'`
        export DST_NODE_PORT=$dst_node_port
        echo "Getting destination Host IP"
        dst_host_ip=`oc get po $VM_NAME-dst-replicator -n $NAMESPACE --kubeconfig $DST_KUBECONFIG -o=jsonpath='{.status.hostIP}'`
        export DST_HOST_IP=$dst_host_ip
        echo "Starting initial volume replication"
        oc exec $VM_NAME-src-replicator -ti -n $NAMESPACE --kubeconfig $SRC_KUBECONFIG -- /bin/bash -c "mkdir /data/dimg; sshfs -o StrictHostKeyChecking=no -o port=$dst_node_port $dst_host_ip:/data/simg /data/dimg; cp -p --sparse=always /data/simg/disk.img /data/dimg/ & progress -m"
        echo "Creating CronJob for async replication"
        yq e -i '.metadata.name = env(VM_NAME)+"-repl-cronjob"' manifests/src-cronjob.yaml
        yq e -i '.spec.jobTemplate.spec.template.spec.containers[0].command[2]="mkdir /data/dimg /data/dfs /data/sfs/; sshfs -o StrictHostKeyChecking=no -o port="+env(DST_NODE_PORT)+" "+ env(DST_HOST_IP)+":/data/simg /data/dimg; guestmount -a /data/simg/disk.img -m /dev/sda4 --ro /data/sfs; guestmount -a /data/dimg/disk.img -m /dev/sda4 --rw /data/dfs; rclone sync --progress /data/sfs/ /data/dfs/ --skip-links --checkers 8 --contimeout 100s --timeout 300s --retries 3 --low-level-retries 10 --drive-acknowledge-abuse --stats 1s --cutoff-mode=soft; sleep 20"' manifests/src-cronjob.yaml;
        yq e -i '.spec.jobTemplate.spec.template.spec.volumes[0].persistentVolumeClaim.claimName = env(VM_NAME)' manifests/src-cronjob.yaml
        yq e -i '.spec.jobTemplate.spec.template.spec.volumes[1].secret.secretName = env(VM_NAME)+"-repl-ssh-keys"' manifests/src-cronjob.yaml
        oc apply -n $NAMESPACE --kubeconfig $SRC_KUBECONFIG -f manifests/src-cronjob.yaml
    fi
fi