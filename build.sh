#! bin/bash

#Build web UI

cd ~/work/github.com/Rafael-hwb/streamhub/web
go install
cp ~/work/bin/web ~/work/bin/streamhub_web_ui/web
cp -R ~/work/src/github.com/Rafael-hwb/streamhub/templates ~/sork/bin/streamhub_web_ui/