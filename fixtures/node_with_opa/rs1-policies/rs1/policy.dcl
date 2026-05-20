POLICY flightRead {
	GRANT read ON flights where destinationCountry = 'DE';
}

POLICY flightRead_withRestrict {
	GRANT read ON flights where destinationCountry IS RESTRICTED;
}