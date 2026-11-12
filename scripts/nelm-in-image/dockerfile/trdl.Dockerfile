FROM alpine:3.24@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf
ARG group
ARG channel
ARG required_version
ARG TARGETARCH

RUN apk add curl gnupg

RUN curl -sSLO "https://tuf.trdl.dev/targets/releases/0.7.0/linux-${TARGETARCH}/bin/trdl" -O "https://tuf.trdl.dev/targets/signatures/0.7.0/linux-${TARGETARCH}/bin/trdl.sig" && \
    curl -sSL https://trdl.dev/trdl.asc | gpg --import && \
    gpg --verify trdl.sig trdl && \
    rm trdl.sig && \
    chmod +x ./trdl && \
    mv trdl /usr/local/bin/trdl && \
    trdl add nelm https://tuf.nelm.sh 1 2122fb476c48de4609fe6d3636759645996088ff6796857fc23ba4b8331a6e3a58fc40f1714c31bda64c709ef6f49bcc4691d091bad6cb1b9a631d8e06e1f308

RUN until echo "Perform trdl update for nelm $group $channel ..." && \
          trdl update nelm ${group} ${channel} && \
          . $(trdl use nelm ${group} ${channel}) && \
          DOWNLOADED_VERSION=$(nelm version | sed -n -e 's|^full: v\{0,1\}||p') && \
          echo "nelm ${group} ${channel}: required version ${required_version}, downloaded version $DOWNLOADED_VERSION" && \
          [ "${required_version}" = "$DOWNLOADED_VERSION" ]; do \
        echo "Version mismatch or update failure! Will retry update" && \
        sleep 1; \
    done && \
    cp $(trdl bin-path nelm ${group} ${channel})/nelm /usr/local/bin/nelm
