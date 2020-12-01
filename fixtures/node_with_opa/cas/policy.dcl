SCHEMA {
	salesOrderId: String
}
 
POLICY salesOrderRead {
	GRANT read ON salesOrder where salesOrderId = '123';
}