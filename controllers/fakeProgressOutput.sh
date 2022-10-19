
	#!/usr/bin/env bash

	# This script is used to generate output similar to `dcp`. It takes in a duration and interval
	# that can be adjusted to meet the needs of the test.  The duration is how long the fake data
	# movement operation runs (in seconds) and the interval is the time between each progress output
	# (using sleep).

	DURATION=$1
	INTERVAL=$2
	
	echo "Faking $DURATION progress entries at an interval of $INTERVALs..."
	
	echo "Creating 1 files."
	echo "Copying data."
	
	for (( c=1; c<$DURATION; c++ )); do
		sleep $INTERVAL
		percent=$(awk -v n="$c" -v p="$DURATION" 'BEGIN{printf("%i\n",n/p*100)}')
		echo "Copied $c.000 GiB (${percent}%) in 1.001 secs (4.174 GiB/s) $(($DURATION-$c)) secs left ..."
	done
	
	echo 'this is stderr' >/dev/stderr
	
	sleep $INTERVAL
	echo "Copied $DURATION.000 GiB (100%) in 1.001 secs (4.174 GiB/s) done"