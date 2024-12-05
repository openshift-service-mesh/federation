import ipaddress
import argparse
import sys

def find_last_host_ip(network):
    """
    Get the last host IP address in a given network.

    :param network: The subnet in CIDR notation (e.g., '172.18.0.0/18').
    :return: The first IP address as a string.
    """
    try:
        network = ipaddress.ip_network(network, strict=False)
        last_host_ip = network.broadcast_address - 1
        return str(last_host_ip)
    except ValueError as e:
        print(f"Error: {e}")
        sys.exit(1)

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description='Get the last host IP address in a given network.')
    parser.add_argument('--network', type=str, required=True, help='The network address in CIDR notation (e.g., 172.18.0.0/16)')

    args = parser.parse_args()

    last_ip = find_last_host_ip(args.network)
    print(last_ip)
