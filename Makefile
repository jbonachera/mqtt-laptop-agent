camera:
	GOOS=linux GOARCH=mipsle go build -buildmode=exe -ldflags="-s -w" -o out/agent . && md5sum out/agent &&  gzip -f out/agent
%::
	GOOS=linux GOARCH=mipsle go build -buildmode=exe -ldflags="-s -w" -o out/agent . && cat out/agent| pv  | ssh root@$@ "cat > /system/sdcard/bin/agent"
