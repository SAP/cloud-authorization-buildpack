package testdata

const EnvWithAuthorization = `{
"authorization": [
{
  "binding_guid": "cf5fbc53-f243-4411-a1d3-f6e254ec7bef",
  "binding_name": null,
  "credentials": {
  "object_store": {
  "access_key_id": "myawstestaccesskeyid",
  "bucket": "my-bucket",
  "host": "s3-eu-central-1.amazonaws.com",
  "region": "eu-central-1",
  "secret_access_key": "mysecretaccesskey",
  "uri": "s3://myawstestaccesskeyid:TTe84CgHEQ%mysecretaccesskey@s3-eu-central-1.amazonaws.com/my-bucket",
  "username": "my-username"
  },
  "ui_url": "https://4b0c2b7a-1279-4352-aasdfsadf--asdfasdf-ams-ui.authorization-dev.cfapps.ls.domain",
  "url": "https://ams.cert.cfapps.ls.domain/",
  "value_help_certificate_issuer": "{\"Country\":[\"DE\"],\"Organization\":[\"SAP SE\"],\"Locality\":[\"Walldorf\"],\"CommonName\":\"SAP Cloud Root CA\"}",
  "value_help_certificate_subject": "{\"Country\":[\"DE\"],\"Organization\":[\"SAP SE\"],\"OrganizationalUnit\":[\"SAP Cloud Platform Clients\",\"Canary\"],\"Locality\":[\"AMS\"],\"CommonName\":\"ValueHelpmTLSCert\"}"
  },
  "instance_guid": "my-instance-guid",
  "instance_name": "my-ams-instance",
  "label": "authorization",
  "name": "my-ams-instance",
  "plan": "application",
  "provider": null,
  "syslog_drain_url": null,
  "tags": [],
  "volume_mounts": []
}
]
}`

const EnvWithAuthorizationDev = `{
"authorization-dev": [
{
  "binding_guid": "cf5fbc53-f243-4411-a1d3-f6e254ec7bef",
  "binding_name": null,
  "credentials": {
  "object_store": {
  "access_key_id": "myawstestaccesskeyid",
  "bucket": "my-bucket",
  "host": "s3-eu-central-1.amazonaws.com",
  "region": "eu-central-1",
  "secret_access_key": "mysecretaccesskey",
  "uri": "s3://myawstestaccesskeyid:TTe84CgHEQ%mysecretaccesskey@s3-eu-central-1.amazonaws.com/my-bucket",
  "username": "my-username"
  },
  "ui_url": "https://4b0c2b7a-1279-4352-aasdfsadf--asdfasdf-ams-ui.authorization-dev.cfapps.ls.domain",
  "url": "https://ams.cert.cfapps.ls.domain/",
  "value_help_certificate_issuer": "{\"Country\":[\"DE\"],\"Organization\":[\"SAP SE\"],\"Locality\":[\"Walldorf\"],\"CommonName\":\"SAP Cloud Root CA\"}",
  "value_help_certificate_subject": "{\"Country\":[\"DE\"],\"Organization\":[\"SAP SE\"],\"OrganizationalUnit\":[\"SAP Cloud Platform Clients\",\"Canary\"],\"Locality\":[\"AMS\"],\"CommonName\":\"ValueHelpmTLSCert\"}"
  },
  "instance_guid": "my-instance-guid",
  "instance_name": "my-ams-instance",
  "label": "authorization-dev",
  "name": "my-ams-instance",
  "plan": "application",
  "provider": null,
  "syslog_drain_url": null,
  "tags": [],
  "volume_mounts": []
}
]
}`
