# Stack Architecture

The following is a high level diagram of the stack architecture. The stack acts as a lightweight ingress. We don't use an existing ingress controller so as to allow users to bring their own and most/all ingresses don't do all the protocols we require (HTTP, TCP, UDP). With this setup all Tinkerbell services (tink server, hegel and smee) can be accessed via a single IP.

```shell
                               ┌───────────┐                                
             ┌─────────────────┤  service  ├──────────────────┐             
             │                 └───────────┘                  │             
             │    Port    Protocol    Backend Flow            │             
             │    ----    --------    ------------            │             
             │    50061   TCP         Nginx -> Hegel          │             
             │    42113   TCP         Nginx -> Tink Server    │             
             │    8080    TCP         Nginx                   │             
             │    7171    TCP         Nginx -> Smee           │             
             │    69      UDP         Nginx -> Smee           │             
             │    514     UDP         Nginx -> Smee           │             
             │    67      UDP         DHCP Relay -> Smee      │             
             └────────────────────────────────────────────────┘             
                               ┌───────────┐                                
┌──────────────────────────────┤    pod    ├───────────────────────────────┐
│                              └───────────┘                               │
│                  ┌────────────────────────────────────┐                  │
│                  │          init containers           │                  │
│                  │                                    │                  │
│                  │          ┌──────────────┐          │                  │
│                  │          │              │          │                  │
│                  │          │  broadcast   │          │                  │
│                  │          │listener setup│          │                  │
│                  │          │              │          │                  │
│                  │          │              │          │                  │
│                  │          └──────────────┘          │                  │
│                  │                                    │                  │
│                  └────────────────────────────────────┘                  │
│                                                                          │
│                  ┌────────────────────────────────────┐                  │
│                  │             containers             │                  │
│                  │                                    │                  │
│                  │ ┌──────────────┐  ┌──────────────┐ │                  │
│                  │ │              │  │              │ │                  │
│                  │ │              │  │              │ │                  │
│                  │ │    nginx     │  │  dhcp relay  │ │                  │
│                  │ │              │  │              │ │                  │
│                  │ │              │  │              │ │                  │
│                  │ └──────────────┘  └──────────────┘ │                  │
│                  │         │                 │        │                  │
│                  └─────────┼─────────────────┼────────┘                  │
│                            │                 │                           │
│                            │                 │                           │
└────────────────────────────┼─────────────────┼───────────────────────────┘
               ┌─────────────┼───────────┐     └────┐                       
               │             │           │          │                       
               │             │       7171/TCP       │                       
           50061/TCP     42113/TCP    69/UDP     67/UDP                     
               │             │       514/UDP        │                       
               │             │           │          │                       
               ▼             ▼           ▼          │                       
          ┌─────────┐   ┌─────────┐    ┌─────────┐  │                       
          │         │   │         │    │         │  │                       
          │         │   │  Tink   │    │         │  │                       
          │  Hegel  │   │ Server  │    │  Smee   │◀─┘                       
          │         │   │         │    │         │                          
          │         │   │         │    │         │                          
          └─────────┘   └─────────┘    └─────────┘                          
```