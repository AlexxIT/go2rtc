## StateChange

1. offer = pc.CreateOffer()
2. pc.SetLocalDescription(offer)
3. OnICEGatheringStateChange: gathering
4. OnSignalingStateChange: have-local-offer
*. OnICEGatheringStateChange: complete
5. pc.SetRemoteDescription(answer)
6. OnSignalingStateChange: stable
7. OnICEConnectionStateChange: checking
8. OnICEConnectionStateChange: connected
