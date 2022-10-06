#!/usr/bin/env bash
set -e

IS_MASTER=`grep -c "INSTANCE_TYPE=master" /var/lib/cloud/instance/scripts/part-001`

if [[ $IS_MASTER != 2 ]];
then
  exit 0
fi

MYSQL_CLIENT=/srv/hops/mysql-cluster/ndb/scripts/mysql-client.sh


# create a mysql user to be able to do mysql calls from the test node

$MYSQL_CLIENT -e "CREATE USER 'test'@'%' IDENTIFIED BY 'Test123';"
$MYSQL_CLIENT -e "GRANT ALL ON *.* TO 'test'@'%';"

APIKEY=$(awk -F'"' '/api_key/{print $4}' /var/lib/cloud/instance/scripts/part-001)
$MYSQL_CLIENT hopsworks -e "INSERT INTO variables (id,value,visibility,hide) VALUES ('api_key', '$APIKEY' ,'0','0')"
