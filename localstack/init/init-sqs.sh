#!/bin/sh
set -e

awslocal sqs create-queue --queue-name solidary-donations
