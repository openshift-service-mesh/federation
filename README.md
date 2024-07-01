### About the project

TODO

### Example of run 
./out/federation-controller --meshPeers "spec: 
              remote: 
                addresses: 
                - lb-1234567890.us-east-1.elb.amazonaws.com
                - 192.168.10.56
                ports: 
                  dataPlane: 15443
                  discovery: 15020" --exportedServiceSet "rules: 
          - type: LabelSelector
            labelSelectors:
              - matchLabels:
                  export-service: \"exportServiceGv\"
              - matchExpressions: {}" --importedServiceSet "rules: 
          - type: LabelSelector
            labelSelectors:
              - matchLabels:
                  export-service: \"importServiceGv\"
              - matchExpressions: {}"
