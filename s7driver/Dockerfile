FROM golang:1.18.2

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
COPY tagList.json ./
RUN go mod download

COPY *.go ./

RUN go build -o /main

EXPOSE 8000

CMD [ "/main" ]