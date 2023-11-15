project-root := env_var('PROJECT_ROOT')
cluster-name := env_var('CLUSTER_NAME')

just-test:
    echo "{{cluster-name}}"

create-cluster:
    kind create cluster --name {{cluster-name}}

delete-cluster:
    kind delete cluster --name {{cluster-name}}

get-kubeconfig:
    kind get kubeconfig --name {{cluster-name}} > {{project-root}}/kubeconfig



