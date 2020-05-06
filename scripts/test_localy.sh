docker run -it -p 127.0.0.1:2112:2112 -e AWS_PROFILE -e AWS_DEFAULT_REGION -v ~/.aws:/root/.aws -v ~/.kube/config:/root/.kube/config  $1
