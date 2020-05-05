package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/joho/godotenv"
	"google.golang.org/api/dns/v1"
	"google.golang.org/api/option"
	"log"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type Domain struct {
	domain         string
	topLevelDomain string
	ip             string
}

func main() {
	err := godotenv.Load()
	if err != nil {
		fmt.Printf("Error loading .env file. %v\n", err)
	}

	projectId := getEnvOrFail("PROJECT_ID")
	credentialsJSON := getEnvOrFail("CREDENTIALS_JSON")
	cName := strings.TrimSpace(getEnvOrFail("CNAME"))
	domains := strings.Fields(getEnvOrFail("DOMAINS"))

	var wrongDomains []Domain

	cNameIp, err := lookupIP(cName)
	if err != nil {
		log.Fatal("could not get IP for cName", err)
	}

	for _, domain := range domains {
		domain = strings.TrimSpace(domain)
		lookupDomain := getLookupDomain(domain)

		domainIp, err := lookupIP(lookupDomain)
		if err != nil {
			log.Fatal("Could not get IP for domain: "+domain, err)
		}

		if cNameIp == domainIp == false {
			topLevelDomain, err := getToplevelDomain(domain)
			if err != nil {
				log.Fatalf("Could not get top level of domain. domain: %s", domain)
			}

			wrongDomains = append(wrongDomains, Domain{
				domain:         domain,
				topLevelDomain: topLevelDomain,
				ip:             domainIp,
			})
		}
	}

	if len(wrongDomains) <= 0 {
		fmt.Printf("No wrong Domains")
		os.Exit(0)
	}

	ctx := context.Background()
	service, err := dns.NewService(ctx, option.WithCredentialsJSON([]byte(credentialsJSON)))

	if err != nil {
		log.Fatal("could not create dns service", err)
	}

	getManagedZones, err := service.ManagedZones.List(projectId).Do()
	if err != nil {
		log.Fatal("could not get managed zones ", err)
	}
	for _, managedZone := range getManagedZones.ManagedZones {
		managedZoneId := strconv.FormatUint(managedZone.Id, 10)
		resourceDomain, changeDnsErr := getToplevelDomain(managedZone.DnsName)
		if changeDnsErr != nil {
			log.Fatalf("could not get resouceDomain. ManagedZoneDns: %s, ManagedZoneId: %d", managedZone.DnsName, managedZone.Id)
		}

		currentWrongDomains := make(map[string]bool)
		currentHasWrongDomains := false

		for _, wrongDomain := range wrongDomains {
			if wrongDomain.topLevelDomain == resourceDomain {
				currentHasWrongDomains = true
				currentWrongDomains[wrongDomain.domain] = true
			}
		}

		if currentHasWrongDomains == false {
			continue
		}

		resourceRecordSets, changeDnsErr := service.ResourceRecordSets.List(projectId, managedZoneId).Do()
		if changeDnsErr != nil {
			log.Fatalf("could not get resource record set. ManagedZoneDns: %s, ManagedZoneId: %d", managedZone.DnsName, managedZone.Id)
		}

		var additions []*dns.ResourceRecordSet
		{
		}
		var deletions []*dns.ResourceRecordSet
		{
		}

		for _, rSet := range resourceRecordSets.Rrsets {
			if rSet.Type != "A" {
				continue
			}
			rSetDomain := strings.TrimSuffix(rSet.Name, ".")
			rSetIp := rSet.Rrdatas[0]
			// if ip was already changed in the config but not propagated
			if currentWrongDomains[rSetDomain] == false || cNameIp == rSetIp {
				continue
			}
			fmt.Println(rSetDomain)

			additions = append(additions, &dns.ResourceRecordSet{
				Name: rSetDomain + ".",
				Rrdatas: []string{
					cNameIp,
				},
				Ttl:  300,
				Type: "A",
			})
			deletions = append(deletions, &dns.ResourceRecordSet{
				Name: rSetDomain + ".",
				Rrdatas: []string{
					rSetIp,
				},
				Ttl:  300,
				Type: "A",
			})
		}

		if len(additions) <= 0 {
			continue
		}

		dnsChange := &dns.Change{
			Additions: additions,
			Deletions: deletions,
		}

		_, err = service.Changes.Create(projectId, managedZoneId, dnsChange).Do()
		if err != nil {
			log.Fatalf("cloud not macke change to dns. ManagedZoneDns: %s, ManagedZoneId: %d", managedZone.DnsName, managedZone.Id)
		}
	}
}

func getEnvOrFail(envName string) string {
	envValue, ok := os.LookupEnv(envName)
	if ok == false {
		log.Fatalf("Could not load enviroment variable. EnvName: %s\n", envName)
	}
	return envValue
}

func getLookupDomain(domain string) string {
	if strings.HasPrefix(domain, "*.") {
		return "this-domain-should-not-exist" + strings.TrimPrefix(domain, "*")
	}
	return domain
}

func getToplevelDomain(domain string) (string, error) {
	domain = strings.TrimSuffix(domain, ".")
	topLevelDomainRegex := regexp.MustCompile("\\w+\\.\\w+$")
	match := topLevelDomainRegex.FindStringSubmatch(domain)
	if len(match) <= 0 {
		return "", errors.New("could not get toplevel domain")
	}
	return match[0], nil
}

func lookupIP(domain string) (string, error) {
	lookedUpIPs, err := net.LookupIP(domain)
	if err != nil || len(lookedUpIPs) <= 0 {
		return "", errors.New("Could not get IP for domain: " + domain)
	}
	return lookedUpIPs[0].String(), nil
}
