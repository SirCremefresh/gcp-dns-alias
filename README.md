[![Gitpod ready-to-code](https://img.shields.io/badge/Gitpod-ready--to--code-blue?logo=gitpod)](https://gitpod.io/#https://github.com/SirCremefresh/gcp-dns-alias)

# GCP DNS Alias

A docker image to replicate the heroku alias dns entry on GCP.
https://support.dnsimple.com/articles/alias-record/  
This is done via a GCP Cloud-Run that has a Crone Job of 30min and checks if the CName Ip has changed. 


Cron:
*/30 * * * *
