# ko is (currently) incomptible with cgo, so the AMQP receive adapter is built via Docker.
# ko uses a debian:stretch-slim
FROM debian:stretch-slim
# AS build_stage
ENV DEBIAN_FRONTEND noninteractive
RUN apt-get update
RUN apt-get install -y cmake uuid-dev libssl-dev libsasl2-2 libsasl2-dev libsasl2-modules git gcc python-dev wget libssl-dev libsasl2-2 libsasl2-modules
RUN wget -q https://dl.google.com/go/go1.10.4.linux-amd64.tar.gz && echo fa04efdb17a275a0c6e137f969a1c4eb878939e91e1da16060ce42f02c2ec5ec go1.10.4.linux-amd64.tar.gz | sha256sum -c --quiet && tar -C /usr/local -xzf go1.10.4.linux-amd64.tar.gz && git clone git://git.apache.org/qpid-proton.git
ENV PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/local/go/bin
ENV GOPATH=/qpid-proton/build/go CGO_CFLAGS=-I/qpid-proton/build/c/include CGO_LDFLAGS='-L/qpid-proton/build/c -lssl -lcrypto -lsasl2'
# build static proton libs and hide dynamic libs from cgo
# proton build problems on master with strech, use f53c768 temporarily (pre catch2 test harness)
RUN cd qpid-proton && git checkout f53c768 && mkdir build && cd build && cmake -DBUILD_GO=ON -DBUILD_CPP=OFF -DBUILD_PYTHON=OFF -DBUILD_STATIC_LIBS=ON -DBUILD_SHARED_LIBS=OFF -DCMAKE_BUILD_TYPE=Release .. && make -j2 && mkdir c/unused && mv c/*.so* c/unused && ln c/libqpid-proton-core-static.a c/libqpid-proton-core.a
RUN git clone git://github.com/knative/eventing.git qpid-proton/build/go/src/github.com/knative/eventing
RUN git clone git://github.com/knative/eventing.git qpid-proton/build/go/src/github.com/knative/eventing-sources
COPY cmd qpid-proton/build/go/src/github.com/knative/eventing-sources/cmd/
COPY pkg qpid-proton/build/go/src/github.com/knative/eventing-sources/pkg/

# Discard cmake go artifacts.  Not sure why this step matters.
RUN rm -f qpid-proton/build/go/pkg/linux_amd64/qpid.apache.org/*.a && rm -rf /root/.cache/go-build
RUN cd qpid-proton/build/go/src/github.com/knative/eventing-sources/cmd/amqpsource && go install -x

RUN mv /qpid-proton/build/go/bin/amqpsource /amqpsource

# Insert here 2nd stage build step for thinner image.
# Bonus points if can be done via a small increment to gcr.io/distroless/cc, i.e. sasl libs
# FROM debian:stretch-slim
# ENV DEBIAN_FRONTEND noninteractive
# RUN apt-get update && apt-get install -y \
#   libssl-dev libsasl2-2 libsasl2-modules \
#   && rm -rf /var/lib/apt/lists/*
# COPY --from=build_stage qpid-proton/build/go/bin/amqpsource /amqpsource
ENTRYPOINT ["/amqpsource"]
