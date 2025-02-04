#!/bin/bash

usage() {
    echo "Usage: $0 --vm-name <vm-name> --namespace <namespace> --src-kubeconfig <file> --dst-kubeconfig <file> [--verbose] [--help]"
    echo
    echo "Options:"
    echo "  --vm-name           Virtual machine name (required)"
    echo "  --namespace         Namespace to work on (required)"
    echo "  --src-kubeconfig    Source kubeconfig file path (required)"
    echo "  --dst-kubeconfig    Destination kubeconfig file path (required)"
    echo "  --help              Display this help message and exit"
    exit 1
}

VM_NAME=""
NAMESPACE=""
SRC_KUBECONFIG=""
DST_KUBECONFIG=""
VERBOSE=0
PVC_NAME=""

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
            yq e -i '.spec.running = false' $VM_NAME-vm.yaml
            oc apply --wait -n $NAMESPACE --kubeconfig $DST_KUBECONFIG -f $VM_NAME-vm.yaml
            echo "Waiting for the destination VM to be created ...... "
            c=1
            while [[ $( oc get vm $VM_NAME -n $NAMESPACE --kubeconfig $DST_KUBECONFIG --no-headers | awk '{print $3}') != "Stopped"  ]]
            do
                echo "$i"
                i=$[$i +5]
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

        echo "Generating source replicator SSH key"
        oc exec $VM_NAME-src-replicator -ti -n $NAMESPACE --kubeconfig $SRC_KUBECONFIG -- bash -c "ssh-keygen -t rsa -b 4096 -N '' -f ~/.ssh/id_rsa"
        oc wait pod $VM_NAME-src-replicator -n $NAMESPACE --kubeconfig $SRC_KUBECONFIG --for=condition=Ready --timeout=-1m
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
        echo $dst_node_port
        echo "Getting destination Host IP"
        dst_host_ip=`oc get po $VM_NAME-dst-replicator -n $NAMESPACE --kubeconfig $DST_KUBECONFIG -o=jsonpath='{.status.hostIP}'`
        echo $dst_host_ip
        echo "Suspending CronJob"
        oc patch cronjob $VM_NAME-repl-cronjob -n $NAMESPACE --kubeconfig $SRC_KUBECONFIG -p '{"spec" : {"suspend" : true }}' 
        echo "Stopping source VM"
        virtctl stop $VM_NAME --kubeconfig $SRC_KUBECONFIG
        while [[ $( oc get vm $VM_NAME -n $NAMESPACE --kubeconfig $SRC_KUBECONFIG --no-headers | awk '{print $3}') != "Stopped"  ]]
        do
            echo "$i"
            i=$[$i +5]
            sleep 5
        done
        echo "Creating final replication job"
        oc create job --from=cronjob/$VM_NAME-repl-cronjob $VM_NAME-repl-final-job -n $NAMESPACE --kubeconfig $SRC_KUBECONFIG
        echo "Waiting final replication"
        oc wait job $VM_NAME-repl-final-job -n $NAMESPACE --kubeconfig $SRC_KUBECONFIG --for=condition=complete --timeout=-1m
        echo "Starting destination VM"
        virtctl start $VM_NAME --kubeconfig $DST_KUBECONFIG
        while [[ $( oc get vm $VM_NAME -n $NAMESPACE --kubeconfig $DST_KUBECONFIG --no-headers | awk '{print $3}') != "Running"  ]]
        do
            printf  "#"
            sleep 5
        done
        echo "Deleting final replication job"
        oc delete job $VM_NAME-repl-final-job -n $NAMESPACE --kubeconfig $SRC_KUBECONFIG --wait
        echo "Deleting CronJob"
        oc delete cronjob $VM_NAME-repl-cronjob -n $NAMESPACE --kubeconfig $SRC_KUBECONFIG --wait
        echo "Deleting source Replicator"
        oc delete pod $VM_NAME-src-replicator -n $NAMESPACE --kubeconfig $SRC_KUBECONFIG --wait
        oc delete secret $VM_NAME-repl-ssh-keys -n $NAMESPACE --kubeconfig $SRC_KUBECONFIG --wait
        echo "Deleting destination Replicator"
        oc delete pod $VM_NAME-dst-replicator -n $NAMESPACE --kubeconfig $DST_KUBECONFIG --wait
        oc delete svc $VM_NAME-dst-svc -n $NAMESPACE --kubeconfig $DST_KUBECONFIG --wait
    fi
fi