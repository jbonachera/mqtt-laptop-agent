%::
	GOOS=linux GOARCH=mipsle go build -o out/agent . && cat out/agent| pv  | ssh root@$@ "cat > /system/sdcard/bin/agent"
