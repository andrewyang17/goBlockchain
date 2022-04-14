FROM golang:1.17

WORKDIR /build
ADD . .

RUN apt install bash

RUN make install

ENTRYPOINT ["gc"]
CMD ["-h"]
