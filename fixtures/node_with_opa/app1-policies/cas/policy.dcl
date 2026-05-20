POLICY salesOrderRead {
	GRANT read ON salesOrder where salesOrderId = '123';
}

POLICY salesOrderRead_withRestrict {
	GRANT read ON salesOrder where salesOrderId IS RESTRICTED;
}