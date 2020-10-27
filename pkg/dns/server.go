package dns

import (
	"fmt"

	"github.com/miekg/dns"
	"github.com/sirupsen/logrus"
)

// dnsTTL tells the DNS resolver how long to cache a query before requesting a new one.
const dnsTTL = 60

// Server is a DNS server forwarding A requests to the configured resolver.
type Server struct {
	dns.Server

	resolver *ShadowServiceResolver
	logger   logrus.FieldLogger
}

// NewServer creates and returns a new DNS server.
func NewServer(port int32, resolver *ShadowServiceResolver, logger logrus.FieldLogger) *Server {
	mux := dns.NewServeMux()

	server := &Server{
		logger:   logger,
		resolver: resolver,
		Server: dns.Server{
			Addr:    fmt.Sprintf(":%d", port),
			Net:     "udp",
			Handler: mux,
		},
	}

	mux.HandleFunc(resolver.Domain(), server.handleDNSRequest)

	return server
}

func (s *Server) handleDNSRequest(w dns.ResponseWriter, r *dns.Msg) {
	msg := &dns.Msg{}

	msg.SetReply(r)
	msg.Authoritative = true

	for _, q := range r.Question {
		if q.Qtype != dns.TypeA {
			continue
		}

		ip, err := s.resolver.LookupFQDN(q.Name)
		if err != nil {
			s.logger.Debugf("Unable to resolve %q: %v", q.Name, err)
			continue
		}

		rr, err := dns.NewRR(fmt.Sprintf("%s %d IN A %s", q.Name, dnsTTL, ip))
		if err != nil {
			s.logger.Errorf("Unable to create RR for %q: %v", q.Name, err)
			continue
		}

		msg.Answer = append(msg.Answer, rr)
	}

	if err := w.WriteMsg(msg); err != nil {
		s.logger.Errorf("Unable to write DNS response: %v", err)
	}
}
