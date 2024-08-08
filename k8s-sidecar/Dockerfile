FROM alpine:3.19.1
RUN apk --no-cache add bash jq
RUN apk --no-cache add bash curl
RUN apk --no-cache add bash tar


WORKDIR /ams

RUN adduser -D -u 443 amsnonroot
RUN chown amsnonroot /ams

COPY resources/opa.tar.gz /opa.tar.gz
RUN tar -xzf /opa.tar.gz -C /bin/ --remove-files
RUN chown -R amsnonroot /bin
RUN chmod +x /bin/opa

COPY ./sidecar/start.sh .
COPY ./sidecar/config-template.json .

USER amsnonroot

CMD ["/bin/bash","/ams/start.sh"]
