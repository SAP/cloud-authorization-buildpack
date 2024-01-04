FROM alpine

RUN apk add bash jq
RUN apk add bash tar

COPY resources/opa.tar.gz /opa.tar.gz

RUN tar -xzf /opa.tar.gz -C /bin/ --remove-files

RUN chmod +x /bin/opa

COPY k8s-sidecar/start.sh /

COPY k8s-sidecar/config-template.json /

CMD ["/start.sh"]