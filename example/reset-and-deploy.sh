#!/usr/bin/env bash
set -euo pipefail

# 0. Required env vars for local testing
export TOKEN="xxx"
export SECRET="xxx"
export ZONE="example.com"
export PROVIDER_HOST="0.0.0.0"
export PROVIDER_PORT="8080"
export TXT_OWNER_ID="default"

# 1. Tear down old resources
kubectl delete deployment external-dns-provider --ignore-not-found
kubectl delete service    external-dns-provider --ignore-not-found
kubectl delete secret external-dns-webhook-credentials --ignore-not-found
kubectl delete configmap external-dns-webhook-config       --ignore-not-found
kubectl delete deployment external-dns          --ignore-not-found
kubectl delete sa         external-dns          --ignore-not-found
kubectl delete clusterrole        external-dns  --ignore-not-found
kubectl delete clusterrolebinding external-dns-viewer --ignore-not-found
kubectl delete ingress whoami-ingress --ignore-not-found
kubectl delete ingress alias-test-ingress --ignore-not-found
kubectl delete ingress cname-test-ingress --ignore-not-found
kubectl delete service whoami        --ignore-not-found
kubectl delete deployment whoami     --ignore-not-found
kubectl delete pod curlpod           --ignore-not-found

# 2. Webhook Provider Deployment + Service
cat <<EOF | envsubst | kubectl apply -f -
---
apiVersion: v1
kind: Secret
metadata:
  name: external-dns-webhook-credentials
type: Opaque
stringData:
  token:      "${TOKEN}"
  secret:     "${SECRET}"
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: external-dns-webhook-config
data:
  config.yaml: |
    providerURL:  "${PROVIDER_HOST}"
    port:         "${PROVIDER_PORT}"
    zoneName:     "${ZONE}"
    registryTXT:  true
    txtOwnerID:   "${TXT_OWNER_ID}"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: external-dns-provider
spec:
  replicas: 1
  selector:
    matchLabels:
      app: external-dns-provider
  template:
    metadata:
      labels:
        app: external-dns-provider
    spec:
      containers:
        - name: external-dns-provider
          image: dockerrc.sakuracr.jp/external-dns-sacloud-webhook:latest
          args:
            - "--token=\$(TOKEN)"
            - "--secret=\$(SECRET)"
            - "--config=/etc/config/config.yaml"
          env:
            - name: TOKEN
              valueFrom:
                secretKeyRef:
                  name: external-dns-webhook-credentials
                  key: token
            - name: SECRET
              valueFrom:
                secretKeyRef:
                  name: external-dns-webhook-credentials
                  key: secret
            - name: WEBHOOK_CONFIG
              value: "/etc/config/config.yaml"
          volumeMounts:
            - name: config
              mountPath: /etc/config
              readOnly: true
          ports:
            - containerPort: ${PROVIDER_PORT}
      volumes:
        - name: config
          configMap:
            name: external-dns-webhook-config
            items:
              - key: config.yaml
                path: config.yaml
---
apiVersion: v1
kind: Service
metadata:
  name: external-dns-provider
spec:
  selector:
    app: external-dns-provider
  ports:
    - name: http
      port: ${PROVIDER_PORT}
      targetPort: ${PROVIDER_PORT}
EOF

kubectl rollout status deployment/external-dns-provider

# 3. External-DNS Controller RBAC + Deployment
cat <<EOF | envsubst | kubectl apply -f -
---
# ServiceAccount for External-DNS
apiVersion: v1
kind: ServiceAccount
metadata:
  name: external-dns
  namespace: default
---
# ClusterRole: Allow reading Services, Endpoints, and Ingresses
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: external-dns
rules:
  - apiGroups: [""]
    resources: ["services", "endpoints"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["networking.k8s.io"]
    resources: ["ingresses"]
    verbs: ["get", "list", "watch"]

---
# ClusterRoleBinding: Bind ClusterRole to ServiceAccount
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: external-dns-viewer
subjects:
  - kind: ServiceAccount
    name: external-dns
    namespace: default
roleRef:
  kind: ClusterRole
  name: external-dns
  apiGroup: rbac.authorization.k8s.io

---
# External-DNS Controller Deployment
apiVersion: apps/v1
kind: Deployment
metadata:
  name: external-dns
spec:
  replicas: 1
  selector:
    matchLabels:
      app: external-dns
  template:
    metadata:
      labels:
        app: external-dns
    spec:
      serviceAccountName: external-dns
      containers:
        - name: external-dns
          image: registry.k8s.io/external-dns/external-dns:v0.18.0
          args:
            - --log-level=debug
            - --source=ingress
            - --provider=webhook
            - --webhook-provider-url=http://external-dns-provider.default.svc.cluster.local:${PROVIDER_PORT}
            - --domain-filter=${ZONE}
            - --registry=txt
            - --txt-prefix=_external-dns.
            - --txt-owner-id=${TXT_OWNER_ID}
            - --policy=sync
            - --annotation-filter=external-dns.alpha.kubernetes.io/managed=true
EOF

kubectl rollout status deployment/external-dns

# 4. Sample App + Ingress
cat <<EOF | envsubst | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: whoami
spec:
  replicas: 1
  selector:
    matchLabels:
      app: whoami
  template:
    metadata:
      labels:
        app: whoami
    spec:
      containers:
        - name: whoami
          image: hashicorp/http-echo:0.2.3
          args: ["-text=hello"]
          ports:
            - containerPort: 5678
---
apiVersion: v1
kind: Service
metadata:
  name: whoami
spec:
  selector:
    app: whoami
  ports:
    - port: 80
      targetPort: 5678
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: whoami-ingress
  annotations:
    external-dns.alpha.kubernetes.io/target: "123.123.123.123"
    external-dns.alpha.kubernetes.io/managed: "true"
spec:
  rules:
    - host: test.${ZONE}
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: whoami
                port:
                  number: 80
EOF


# 4a. CNAME Record
cat <<EOF | envsubst | kubectl apply -f -
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: cname-test-ingress
  annotations:
    external-dns.alpha.kubernetes.io/target: "test.example.com."
    external-dns.alpha.kubernetes.io/record-type: "CNAME"
    external-dns.alpha.kubernetes.io/managed: "true"
    external-dns.alpha.kubernetes.io/ttl: "120"
spec:
  rules:
    - host: cname-test.${ZONE}
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: whoami
                port:
                  number: 80
EOF

# 4b. ALIAS Record 
cat <<EOF | envsubst | kubectl apply -f -
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: alias-test-ingress
  annotations:
    external-dns.alpha.kubernetes.io/target: "test.example.com."
    external-dns.alpha.kubernetes.io/record-type: "ALIAS"
    external-dns.alpha.kubernetes.io/managed: "true"
    external-dns.alpha.kubernetes.io/ttl: "10"
spec:
  rules:
    - host: alias-test.${ZONE}
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: whoami
                port:
                  number: 80
EOF
