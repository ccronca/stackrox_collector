#!/usr/bin/env bash

set -e
REGION=us-central1

#zones=$(gcloud compute zones list --filter="region=$REGION" | grep UP | cut -f1 -d' ')
success=false
for zone in us-central1-a us-central1-b ; do
    echo "Trying zone $zone"
    gcloud config set compute/zone "${zone}"
    if gcloud compute instances create \
      --image-family ubuntu-1804-lts \
      --image-project ubuntu-os-cloud \
      --service-account=circleci-collector@stackrox-ci.iam.gserviceaccount.com \
        "collector-nb-${CIRCLE_BUILD_NUM}"
    then
				success=true
        break
    else
        gcloud compute instances delete "collector-nb-${CIRCLE_BUILD_NUM}"
    fi
done

if test ! "$success" = "true" ; then
  echo "Could not boot instance."
  exit 1
fi

echo "A000"
sleep 30  # give it time to boot
cd /tmp
echo "A001"
gitdir=${SOURCE_ROOT}
git clone $gitdir shipdir
echo "A002"
rm -rf shipdir/.git
echo $CIRCLE_BUILD_NUM > shipdir/buildnum.txt
echo "A003"
mkdir s2
mv shipdir s2/collector
echo "A004"
cd s2
tar cvfz collector.tar.gz collector/
cd ..
mv s2/collector.tar.gz .
echo "A005"
rm -rf s2
gcloud compute scp collector.tar.gz "collector-nb-${CIRCLE_BUILD_NUM}":
echo "A007"
gcloud compute ssh "collector-nb-${CIRCLE_BUILD_NUM}" --command "pwd && ls -Fh"
echo "A008"
exit 0
