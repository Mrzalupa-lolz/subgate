// Optional isolated collector for nDPI 4.14.x; 4.2.x is observation-only for
// BitTorrent because that version reports even signatures as cache guesses.
// Packet payload stays in this process. Only bounded flow metadata is emitted.
#include <ndpi_api.h>
#include <pcap/pcap.h>
#include <arpa/inet.h>
#include <signal.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <unistd.h>
#include <grp.h>
#include <linux/capability.h>
#include <sys/prctl.h>
#include <sys/syscall.h>

#define LIMIT 2000
struct entry {
  unsigned char key[37];
  struct ndpi_flow_struct *flow;
  ndpi_protocol protocol;
  uint64_t id, bytes, packets, last;
  uint32_t syn_sequence;
  int saw_syn;
  int used, dirty;
};
static struct entry entries[LIMIT];
static struct ndpi_detection_module_struct *module;
static uint64_t serial, lost;
static int truncated;
static pcap_t *active_capture;
static int offline_capture;
static volatile sig_atomic_t stop;
static void stopping(int signo) { (void)signo; stop=1; }

static void release(struct entry *e) {
  if(e->flow) ndpi_flow_free(e->flow);
  memset(e,0,sizeof(*e));
}

static void emit(pcap_t *capture, uint64_t now, int offline) {
  struct pcap_stat stats;
  if(!offline && pcap_stats(capture,&stats)==0 && stats.ps_drop>lost) lost=stats.ps_drop;
  printf("{\"dropped\":%llu,\"truncated\":%s,\"flows\":[",(unsigned long long)lost,truncated?"true":"false");
  int comma=0;
  for(int i=0;i<LIMIT;i++) {
    struct entry *e=&entries[i];
    if(!e->used) continue;
    if(e->dirty) {
      char name[128], timestamp[40];
      time_t seconds=(time_t)(e->last/1000);
      struct tm tm; gmtime_r(&seconds,&tm);
      strftime(timestamp,sizeof(timestamp),"%Y-%m-%dT%H:%M:%SZ",&tm);
      ndpi_protocol2name(module,e->protocol,name,sizeof(name));
      // Protocol names originate in libnDPI, but still escape JSON metacharacters.
      for(char *p=name;*p;p++) if(*p=='"'||*p=='\\'||(unsigned char)*p<32) *p='_';
      const char *confidence=e->flow->confidence==NDPI_CONFIDENCE_DPI?"dpi":e->flow->confidence==NDPI_CONFIDENCE_UNKNOWN?"unknown":"guessed";
      printf("%s{\"id\":\"%llu\",\"protocol\":\"%s\",\"confidence\":\"%s\",\"version\":\"%s\",\"timestamp\":\"%s\",\"bytes\":%llu,\"packets\":%llu,\"dropped\":%llu,\"truncated\":%s}",comma?",":"",(unsigned long long)e->id,name,confidence,ndpi_revision(),timestamp,(unsigned long long)e->bytes,(unsigned long long)e->packets,(unsigned long long)lost,truncated?"true":"false");
      comma=1; e->dirty=0;
    }
    if(now>e->last+120000) release(e);
  }
  puts("]}"); fflush(stdout);
}

static void packet(const struct pcap_pkthdr *h,const unsigned char *data) {
  size_t off=14;
  if(h->caplen<off) {lost++;return;}
  uint16_t ether=(uint16_t)(data[12]<<8|data[13]);
  for(int i=0;i<2&&(ether==0x8100||ether==0x88a8);i++) { if(h->caplen<off+4){lost++;return;} ether=(uint16_t)(data[off+2]<<8|data[off+3]);off+=4; }
  if(ether!=0x0800&&ether!=0x86dd) return;
  const unsigned char *ip=data+off;
  size_t len=h->caplen-off, transport, declared;
  unsigned char key[37]={0},source[18]={0},dest[18]={0},proto;
  if(ether==0x0800) {
    if(len<20||ip[0]>>4!=4){lost++;return;}
    transport=(ip[0]&15)*4;declared=(size_t)(ip[2]<<8|ip[3]);
    // Fragmented captures cannot prove classification/attribution completeness.
    if(transport<20 || (ip[6]&0x3f)||ip[7]){lost++;return;}
    proto=ip[9];memcpy(source+12,ip+12,4);memcpy(dest+12,ip+16,4);key[0]=4;
  } else {
    if(len<40||ip[0]>>4!=6){lost++;return;}
    transport=40;declared=40+(size_t)(ip[4]<<8|ip[5]);proto=ip[6];
    memcpy(source,ip+8,16);memcpy(dest,ip+24,16);key[0]=6;
    // Do not guess a tuple through fragments or unknown extension headers.
    if(proto!=6&&proto!=17){lost++;return;}
  }
  if(proto!=6&&proto!=17) return;
  if(declared>len||declared<transport+(proto==6?20:8)||declared>65535){lost++;return;}
  memcpy(source+16,ip+transport,2);memcpy(dest+16,ip+transport+2,2);
  if(memcmp(source,dest,18)>0){memcpy(key+1,dest,18);memcpy(key+19,source,18);}else{memcpy(key+1,source,18);memcpy(key+19,dest,18);}
  // Version and protocol both belong to the key (IPv4/IPv6, TCP/UDP).
  key[0]=(unsigned char)(key[0]*32+proto);
  struct entry *e=NULL,*empty=NULL;
  for(int i=0;i<LIMIT;i++){if(entries[i].used&&memcmp(entries[i].key,key,sizeof(key))==0){e=&entries[i];break;}if(!entries[i].used&&!empty)empty=&entries[i];}
  uint64_t now=(uint64_t)h->ts.tv_sec*1000+(uint64_t)h->ts.tv_usec/1000;
  int syn=proto==6&&(ip[transport+13]&0x12)==0x02;
  uint32_t sequence=0;if(syn)memcpy(&sequence,ip+transport+4,4);
  if(e && ((syn&&(!e->saw_syn||e->syn_sequence!=sequence)) || now>e->last+(proto==17?30000:120000))) {
    // A reused tuple is a new flow, with fresh classifier state and ID.
    // Flush metadata before resetting so counters are not silently lost.
    emit(active_capture,now,offline_capture);release(e);empty=e;e=NULL;
  }
  if(!e) {
    if(!empty){truncated=1;return;}e=empty;
    e->flow=ndpi_flow_malloc(SIZEOF_FLOW_STRUCT);if(!e->flow){truncated=1;return;}
    memset(e->flow,0,SIZEOF_FLOW_STRUCT);memcpy(e->key,key,sizeof(key));e->used=1;e->id=++serial;
  }
  if(syn){e->saw_syn=1;e->syn_sequence=sequence;}
  e->last=(uint64_t)h->ts.tv_sec*1000+(uint64_t)h->ts.tv_usec/1000;e->bytes+=declared;e->packets++;e->dirty=1;
  if(e->packets<=256) {
#ifdef SUBGATE_NDPI_414
    e->protocol=ndpi_detection_process_packet(module,e->flow,ip,(uint16_t)declared,e->last,NULL);
#else
    e->protocol=ndpi_detection_process_packet(module,e->flow,ip,(uint16_t)declared,e->last);
#endif
  }
}

int main(int argc,char **argv) {
  if(argc!=3 || (strcmp(argv[1],"--interface")&&strcmp(argv[1],"--pcap"))){fprintf(stderr,"usage: subgate-ndpi --interface IFACE | --pcap FILE\n");return 2;}
  int offline=!strcmp(argv[1],"--pcap");char error[PCAP_ERRBUF_SIZE];
  pcap_t *capture=offline?pcap_open_offline(argv[2],error):pcap_open_live(argv[2],65535,0,1000,error);
  if(!capture){fprintf(stderr,"capture unavailable\n");return 1;}
  active_capture=capture;offline_capture=offline;
  if(pcap_datalink(capture)!=DLT_EN10MB){fprintf(stderr,"Ethernet capture required\n");pcap_close(capture);return 1;}
#ifdef SUBGATE_NDPI_414
  module=ndpi_init_detection_module(NULL);
#else
  module=ndpi_init_detection_module(ndpi_no_prefs);
#endif
  if(!module){pcap_close(capture);return 1;}
  NDPI_PROTOCOL_BITMASK mask;NDPI_BITMASK_SET_ALL(mask);ndpi_set_protocol_detection_bitmask2(module,&mask);ndpi_finalize_initialization(module);
  // The parent worker may manage TC. The capture child drops those rights
  // after opening libpcap and cannot regain them or execute privileged code.
  if(geteuid()==0 && (setgroups(0,NULL)!=0 || setgid(65534)!=0 || setuid(65534)!=0)){fprintf(stderr,"privilege drop failed\n");return 1;}
  struct __user_cap_header_struct cap_header={_LINUX_CAPABILITY_VERSION_3,0};
  struct __user_cap_data_struct cap_data[2]={{0}};
  if(syscall(SYS_capset,&cap_header,&cap_data)!=0 || prctl(PR_SET_NO_NEW_PRIVS,1,0,0,0)!=0){fprintf(stderr,"capability drop failed\n");return 1;}
  signal(SIGTERM,stopping);signal(SIGINT,stopping);
  uint64_t previous=(uint64_t)time(NULL)*1000;
  if(!offline && pcap_setnonblock(capture,1,error)!=0){fprintf(stderr,"nonblocking capture required\n");pcap_close(capture);ndpi_exit_detection_module(module);return 1;}
  while(!stop) {
    struct pcap_pkthdr *header;const unsigned char *data;int result=pcap_next_ex(capture,&header,&data);
    uint64_t now=(uint64_t)time(NULL)*1000;
    if(result==1) {packet(header,data);if(offline)now=(uint64_t)header->ts.tv_sec*1000;}
    else if(result==-2)break;
    else if(result==-1){fprintf(stderr,"capture read failed\n");lost++;break;}
    else usleep(1000);
    if(!offline&&now>=previous+10000){emit(capture,now,0);previous=now;}
  }
  emit(capture,UINT64_MAX,offline);
  for(int i=0;i<LIMIT;i++)release(&entries[i]);
  ndpi_exit_detection_module(module);pcap_close(capture);return lost||truncated?1:0;
}
