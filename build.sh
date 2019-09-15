#!/bin/bash

# TODO: test docker images before push

# Log output:

DATE_VERSION=$(date +%Y-%b-%d_%02Hh%02Mm%02S)
APP_BIN=/app/demo-binary

mkdir -p logs
exec 2>&1 > >( stdbuf -oL tee logs/${0}.${DATE_VERSION}.log )

# -- Functions: --------------------------------------------------------

function die {
    echo "$0: die - $*" >&2
    exit 1
}

function check_build {
    SRC_GO=$1; shift

    [ ! -f $SRC_GO ] && die "No such src file <$SRC_GO>"

    echo; echo "---- Building binary ----------"
    [ -f demo-binary ] && rm -f demo-binary ]
    CGO_ENABLED=0 go build -a -o demo-binary $SRC_GO
    [ ! -x demo-binary ] && die "Failed to build binary"
    ls -alh demo-binary

    echo; echo "---- Checking binary version ----------"
    ./demo-binary --version |&
        grep $DATE_VERSION || die "Bad version != $DATE_VERSION"

    echo; echo "---- Testing  binary ----------"
    LISTEN=127.0.0.1:8080
    ./demo-binary --listen $LISTEN &
    [ $? -ne 0 ] && die "Failed to launch binary"
    PID=$!
    [ -z "$PID" ] && die "Failed to get PID"
    ps -fade | grep $PID

    curl -sL $LISTEN/1 || {
        kill -9 $PID
        die "Failed to contact demo-binary on <$LISTEN>"
    }
    kill -9 $PID
    echo "---- binary OK ----------------"
}

# Properly cached 2stage_build:
#   See https://pythonspeed.com/articles/faster-multi-stage-builds/
function build {
    IMAGE_TAG=$1; shift
    FROM_IMAGE=$1; shift
    EXPOSE_PORT=$1; shift
    TEMPLATE_CMD="$*"; set --

    IMAGE_NAME_VERSION=$IMAGE_TAG
    IMAGE_VERSION=${IMAGE_TAG#*:}

    template_go_src main.go main.build.go

    [ "$TEMPLATE_CMD" = "CMD" ] && die "build: Missing command in <$TEMPLATE_CMD>"
    write_dockerfile $IMAGE_TAG $FROM_IMAGE $EXPOSE_PORT "$TEMPLATE_CMD"

    case "$FROM_IMAGE" in
        "scratch") STAGE1_IMAGE="mjbright/demo-static-binary";;
        "alpine")  STAGE1_IMAGE="mjbright/demo-dynamic-binary";;

        *)  die "Unknown FROM_IMAGE type <$FROM_IMAGE>";;
    esac

    #set -euo pipefail

    # Pull the latest version of the image, in order to populate the build cache:
    TIME docker pull $STAGE1_IMAGE || true
    TIME docker pull $IMAGE_TAG    || true

    # Build the compile stage:
    TIME docker build --target build-env     --cache-from=$STAGE1_IMAGE --tag $STAGE1_IMAGE . ||
        die "Build failed"

    # Build the runtime stage, using cached compile stage:
    TIME docker build --target runtime-image \
                 --cache-from=$STAGE1_IMAGE \
                 --cache-from=$IMAGE_TAG --tag $IMAGE_TAG . || die "Build failed"
    echo "CMD=<$TEMPLATE_CMD>"
    [ "$TEMPLATE_CMD" = "CMD" ] && die "Missing command in <$TEMPLATE_CMD>"

    echo; echo "---- [docker] Checking $IMAGE_TAG command ----------"
    docker history --no-trunc $IMAGE_TAG | awk '/CMD / { FS="CMD"; $0=$0; print "CMD",$2; } '

    ITAG=$(echo $IMAGE_TAG | sed 's?[/:_]?-?g')
    echo; echo "---- [docker] Checking $IMAGE_TAG version ----------"
    docker rm --force name versiontest-$ITAG 2>/dev/null
    docker run --rm --name versiontest-$ITAG mjbright/ckad-demo:1 $APP_BIN --version |&
        grep $DATE_VERSION || die "Bad version != $DATE_VERSION"

    echo; echo "---- [docker] Testing  $IMAGE_TAG ----------"
    let DELAY=LIVE+READY
    [ $DELAY -ne 0 ] && { echo "Waiting for live/ready $LIVE/$READY secs"; sleep $DELAY; }

    CONTAINERNAME=buildtest-$ITAG
    docker run --rm -d --name $CONTAINERNAME -p 8181:$EXPOSE_PORT $IMAGE_TAG
    CONTAINERID=$(docker ps -ql)
    curl -sL 127.0.0.1:8181/1 ||
      die "Failed to interrogate container <$CONTAINERID> $CONTAINERNAME from image <$IMAGE_TAG>"
    docker stop $CONTAINERID
    #docker rm $CONTAINERID

    # Push the new versions:
    TIME docker push $STAGE1_IMAGE
    TIME docker push $IMAGE_TAG

    test_kubernetes_images 
}

function test_kubernetes_images {
    # NO USE as this CAN ONLY BE DONE AFTER push

    JOBNAME=kubejobtest-$ITAG
    # Prints to log, but difficult to manage, keeps restarting Pod
    #kubectl run --rm --image-pull-policy '' --generator=run-pod/v1 --image=mjbright/ckad-demo:1 testerckad -it -- -v -die
    # Don't want --image-pull-policy '' as this will force pull from .... docker hub!!

    echo; echo "---- [kubernetes] Checking $IMAGE_TAG version ----------"
    kubectl delete job $JOBNAME 2>/dev/null
    TIME kubectl create job --image=$IMAGE_TAG $JOBNAME -- $APP_BIN --version ||
	    die "Failed to create job <$JOBNAME>"
    MAX_LOOPS=10
    while ! kubectl get jobs/$JOBNAME | grep "1/1"; do
        let MAX_LOOPS=MAX_LOOPS-1; [ $MAX_LOOPS -eq 0 ] && die "Stopping ..."
        echo "Waiting for job to complete ..."; sleep 2;

    done

    kubectl logs jobs/$JOBNAME |& grep $DATE_VERSION || { 
        kubectl delete jobs/$JOBNAME;
        die "Bad version != $DATE_VERSION";
    }
    kubectl delete jobs/$JOBNAME

    echo; echo "---- [kubernetes] Testing  $IMAGE_TAG ----------"
    let DELAY=LIVE+READY
    [ $DELAY -ne 0 ] && { echo "Waiting for live/ready $LIVE/$READY secs"; sleep $DELAY; }

    #kubectl run --rm --generator=run-pod/v1 --image=mjbright/ckad-demo:1 testerckad -it -- --listen 127.0.0.1:80
    PODNAME=kubetest-$ITAG
    kubectl run --generator=run-pod/v1 --image=$IMAGE_TAG $PODNAME -- --listen 127.0.0.1:80

    MAX_LOOPS=10
    while ! kubectl get pods/$PODNAME | grep "Running"; do
        let MAX_LOOPS=MAX_LOOPS-1; [ $MAX_LOOPS -eq 0 ] && die "Stopping ..."
        echo "Waiting for pod to reach Running state ..."; sleep 1;
    done

    kubectl port-forward pod/$PODNAME 8181:80 &
    PID=$!
    sleep 2

    curl -sL 127.0.0.1:8181/1 || {
        kill -9 $PID
        kubectl delete pod/$PODNAME
        die "Failed to interrogate pod <$PODNAME> from image <$IMAGE_TAG>"
    }

    # NEED TO KILL POD
    kill -9 $PID
    kubectl delete pod/$PODNAME

}

function write_dockerfile {
    IMAGE_TAG=$1; shift
    FROM_IMAGE=$1; shift
    EXPOSE_PORT=$1; shift
    TEMPLATE_CMD="$*"; set --

    #echo "EXPOSE_PORT=$EXPOSE_PORT"
    [ "$TEMPLATE_CMD" = "CMD" ] && die "write_dockerfile: Missing command in <$TEMPLATE_CMD>"

    STATIC_STAGE1_BUILD="CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -ldflags '-w' -o demo-binary main.build.go"
    DYNAMIC_STAGE1_BUILD="CGO_ENABLED=0 go build -a -o demo-binary main.build.go"

    case "$FROM_IMAGE" in
        "scratch") STAGE1_BUILD=$STATIC_STAGE1_BUILD;;
        "alpine")  STAGE1_BUILD=$DYNAMIC_STAGE1_BUILD;;
        *)  die "Unknown FROM_IMAGE type <$FROM_IMAGE>";;
    esac

    sed  < Dockerfile.tmpl > Dockerfile \
        -e "s/__FROM_IMAGE__/$FROM_IMAGE/" \
        -e "s/__EXPOSE_PORT__/$EXPOSE_PORT/" \
        -e "s/__STAGE1_BUILD__/$STAGE1_BUILD/" \
        -e "s?__TEMPLATE_CMD__?$TEMPLATE_CMD?" \

    grep __ Dockerfile && die "Uninstantiated variables in Dockerfile"
    mkdir -p tmp

    DFID=$(echo $IMAGE_TAG | sed -e 's/\//_/g')
    cp -a Dockerfile tmp/Dockerfile.${DFID}
    cp -a main.build.go tmp/main.build.go.${DFID}
}

function basic_2stage_build {
    IMAGE_TAG=$1; shift
    FROM_IMAGE=$1; shift
    EXPOSE_PORT=$1; shift
    TEMPLATE_CMD="$*"; set --

    write_dockerfile $IMAGE_TAG $FROM_IMAGE $EXPOSE_PORT "$TEMPLATE_CMD"
    docker build -t $IMAGE_TAG .
}

function push {
    IMAGE_TAG=$1; shift
    FROM_IMAGE=$1; shift

    docker push $IMAGE_TAG 
}

function build_and_push {
    #IMAGE_TAG=$1; shift
    #build $IMAGE_TAG
    #push $IMAGE_TAG

    #[ "$TEMPLATE_CMD" = "CMD" ] && die "build: Missing command in <$TEMPLATE_CMD>"
    echo $4
    build $*
    push  $*
}

# START: TIMER FUNCTIONS ================================================

function TIMER_START { START_S=`date +%s`; }

function TIMER_STOP {
    END_S=`date +%s`
    let TOOK=END_S-START_S

    TIMER_hhmmss $TOOK
    return 0
}

function TIME {
    CMD=$*

    CMD_TIME=$(date +%Y-%b-%d_%02Hh%02Mm%02S)
    echo; echo "---- [$CMD_TIME] $CMD"
    TIMER_START
    $CMD; RET=$?
    TIMER_STOP
    echo "Took $TOOK secs [${HRS}h${MINS}m${SECS}]"
    [ $RET -ne 0 ] && echo "ERROR: returned $RET"
    return $RET
}

function TIMER_hhmmss {
    _REM_SECS=$1; shift
    let SECS=_REM_SECS%60
    let _REM_SECS=_REM_SECS-SECS
    let MINS=_REM_SECS/60%60
    let _REM_SECS=_REM_SECS-60*MINS
    let HRS=_REM_SECS/3600

    [ $SECS -lt 10 ] && SECS="0$SECS"
    [ $MINS -lt 10 ] && MINS="0$MINS"
    return 0
}

function template_go_src {
    SRC=$1;       shift
    BUILD_SRC=$1; shift

    sed < ${SRC} > ${BUILD_SRC}  \
           -e "s/TEMPLATE_DATE_VERSION/$DATE_VERSION/" \
           -e "s?TEMPLATE_IMAGE_NAME_VERSION?$IMAGE_NAME_VERSION?" \
           -e "s/TEMPLATE_IMAGE_VERSION/$IMAGE_VERSION/" \

    ls -altr ${SRC} ${BUILD_SRC}
    [ ! -s ${BUILD_SRC} ] && die "Empty ${BUILD_SRC} !!"
}

function build_and_push_tags {
    for REPO_NAME in $REPO_NAMES; do
        echo; echo "---- Building images <$REPO_NAME> --------"
        for TAG in $TAGS; do
            REPO="mjbright/$REPO_NAME"
            PORT=80

            IMAGE="${REPO}:${TAG}"
            CMD="[\"$APP_BIN\",\"--listen\",\":$PORT\",\"-l\",\"$LIVE\",\"-r\",\"$READY\"]"
            build_and_push $IMAGE scratch $PORT $CMD

            IMAGE="${REPO}:alpine${TAG}"
            CMD="[\"$APP_BIN\",\"--listen\",\":$PORT\",\"-l\",\"$LIVE\",\"-r\",\"$READY\"]"
            build_and_push $IMAGE alpine  $PORT $CMD

            ## IMAGE="${REPO}:bad${TAG}"
            ## build_and_push $IMAGE alpine  $PORT  "--listen :$PORT -l 10 -r 10 -i $IMAGE"
        done
    done
}

# END: TIMER FUNCTIONS ================================================

IMAGE_NAME_VERSION=""
IMAGE_VERSION=""

template_go_src main.go main.build.go

TIME check_build main.build.go
#die "OK"

## -- Args: -------------------------------------------------------------

ALL_REPO_NAMES="ckad-demo k8s-demo docker-demo"
ALL_TAGS=$(seq 6)

REPO_NAMES="ckad-demo"
TAGS=""

while [ ! -z "$1" ]; do
    case $1 in
        [0-9]*)           TAGS+=" $1";;
        --tag|-t)         shift; TAGS=$1;;
        --all|-a)         TAGS=$ALL_TAGS; REPO_NAMES=$ALL_REPO_NAMES;;
        --all-tags|-at)   TAGS=$ALL_TAGS;;
        --all-images|-ai) REPO_NAMES=$ALL_REPO_NAMES;;
    esac
    shift
done

[ -z "$TAGS" ] && TAGS="1"

## -- Main: -------------------------------------------------------------

# Incremental builds:

docker login

TIMER_START; START0_S=$START_S

#LIVENESS_DELAY=0
LIVE=0
#READINESS_DELAY=0
READY=0

#LIVE=03
#READY=03

build_and_push_tags

START_S=$START0_S; TIMER_STOP; echo "SCRIPT Took $TOOK secs [${HRS}h${MINS}m${SECS}]"

#build_and_push "mjbright/ckad-demo:alpine1" "alpine" 

